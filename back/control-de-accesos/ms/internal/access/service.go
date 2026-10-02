package access

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image/color"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"

	"controldeaccesos/internal/audit"
	"controldeaccesos/internal/notifier"
	authmw "controldeaccesos/internal/platform/middleware"
)

// qrMaxValidity caps how long a freshly generated QR can stay valid,
// regardless of what the creator requested (FR-05-B). PRE_AUTH grants are
// not subject to this cap.
const qrMaxValidity = 12 * time.Hour

const (
	ActionCreateGrant   = "CREATE_ACCESS_GRANT"
	ActionCancelGrant   = "CANCEL_ACCESS_GRANT"
	ActionValidateGrant = "VALIDATE_ACCESS_GRANT"
	ActionGetGrant      = "GET_ACCESS_GRANT"
)

type Service interface {
	CreateQRCode(ctx context.Context, complexID uuid.UUID, createdBy uuid.UUID, req CreateQRCodeRequest) (*AccessGrantResponse, error)
	CreateQRBatch(ctx context.Context, complexID uuid.UUID, createdBy uuid.UUID, req CreateQRBatchRequest) ([]*AccessGrant, error)
	GetQRCodes(ctx context.Context, complexID uuid.UUID) ([]*AccessGrant, error)
	CreatePreAuth(ctx context.Context, complexID uuid.UUID, createdBy uuid.UUID, req CreatePreAuthRequest) (*AccessGrant, error)
	GetPreAuthorizations(ctx context.Context, complexID uuid.UUID) ([]*AccessGrant, error)

	CreateAccessGrant(ctx context.Context, complexID uuid.UUID, createdBy uuid.UUID, req CreateAccessGrantRequest) (*AccessGrant, error)
	GetAccessGrant(ctx context.Context, complexID uuid.UUID, userID uuid.UUID, id uuid.UUID) (*AccessGrant, error)
	CancelAccessGrant(ctx context.Context, complexID uuid.UUID, userID uuid.UUID, id uuid.UUID) error
	ValidateAccess(ctx context.Context, req ValidateAccessRequest) (*ValidateAccessResponse, error)
	GetUnitAccessGrants(ctx context.Context, unitID uuid.UUID, status *string) ([]*AccessGrant, error)
}

// UnitMembershipChecker verifies a caller is associated with a unit — the
// IDOR guard for every grant-write/read/cancel operation below.
type UnitMembershipChecker interface {
	IsUserAssociatedWithUnit(ctx context.Context, userID, unitID uuid.UUID) (bool, error)
}

type DirectoryRepository interface {
	// GetUnitComplexID's found return is false when the unit simply doesn't
	// exist (vs. err, which signals an actual lookup failure).
	GetUnitComplexID(ctx context.Context, unitID uuid.UUID) (complexID uuid.UUID, found bool, err error)
	GetUnitName(ctx context.Context, unitID uuid.UUID) (string, error)
	GetUserDisplayName(ctx context.Context, userID uuid.UUID) (string, error)
}

// Implementations must never block or fail the caller.
type Notifier interface {
	Notify(ctx context.Context, event notifier.Event)
}

// Auditor persists an append-only audit trail entry.
type Auditor interface {
	Write(ctx context.Context, e audit.Entry) error
}

type service struct {
	repo        Repository
	directory   DirectoryRepository
	unitChecker UnitMembershipChecker
	notifier    Notifier
	auditor     Auditor
	logger      *slog.Logger
}

func NewService(repo Repository, directory DirectoryRepository, unitChecker UnitMembershipChecker, notif Notifier, auditor Auditor, logger *slog.Logger) Service {
	return &service{
		repo:        repo,
		directory:   directory,
		unitChecker: unitChecker,
		notifier:    notif,
		auditor:     auditor,
		logger:      logger,
	}
}

// assertUnitAccess returns ErrUnitAccessDenied unless the user belongs to the
// unit. A nil checker fails closed.
func (s *service) assertUnitAccess(ctx context.Context, userID, unitID uuid.UUID) error {
	if s.unitChecker == nil {
		return ErrUnitAccessDenied
	}
	ok, err := s.unitChecker.IsUserAssociatedWithUnit(ctx, userID, unitID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrUnitAccessDenied
	}
	return nil
}

func (s *service) notify(ctx context.Context, eventType string, grant *AccessGrant) {
	if s.notifier == nil || grant == nil {
		return
	}
	s.notifier.Notify(ctx, notifier.Event{
		Type:        eventType,
		ComplexID:   grant.ComplexID,
		UnitID:      grant.UnitID,
		GrantID:     grant.ID,
		VisitorName: grant.VisitorName,
		Method:      grant.Method,
	})
}

// audit writes a best-effort append-only trail entry. A nil auditor (e.g.
// unit tests) or a write failure never affects the caller — it's only
// logged.
func (s *service) audit(ctx context.Context, actorID, complexID *uuid.UUID, resourceType string, resourceID *uuid.UUID, action, result, reason string) {
	if s.auditor == nil {
		return
	}
	entry := audit.Entry{
		ActorID:       actorID,
		ComplexID:     complexID,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		Action:        action,
		Result:        result,
		Reason:        reason,
		IPAddress:     authmw.GetRequestIP(ctx),
		CorrelationID: authmw.GetCorrelationID(ctx),
	}
	if err := s.auditor.Write(ctx, entry); err != nil {
		s.logger.Error("audit write failed", "error", err, "action", action)
	}
}

func (s *service) validateUnitBelongsToComplex(ctx context.Context, complexID, unitID uuid.UUID) error {
	unitComplexID, found, err := s.directory.GetUnitComplexID(ctx, unitID)
	if err != nil {
		return fmt.Errorf("failed to load unit: %w", err)
	}
	if !found {
		return ErrUnitNotFound
	}
	if unitComplexID != complexID {
		return ErrUnitComplexMismatch
	}
	return nil
}

// checkUnitOwnership runs both IDOR guards (unit belongs to the caller's
// complex, caller belongs to the unit) and audits a DENIED entry on failure.
func (s *service) checkUnitOwnership(ctx context.Context, action string, actorID, complexID, unitID uuid.UUID) error {
	if err := s.validateUnitBelongsToComplex(ctx, complexID, unitID); err != nil {
		s.audit(ctx, &actorID, &complexID, "unit", &unitID, action, audit.ResultDenied, err.Error())
		return err
	}
	if err := s.assertUnitAccess(ctx, actorID, unitID); err != nil {
		s.audit(ctx, &actorID, &complexID, "unit", &unitID, action, audit.ResultDenied, err.Error())
		return err
	}
	return nil
}

// clampQRExpiry enforces qrMaxValidity: whichever of the creator's requested
// expiry or the 12h cap comes first wins.
func clampQRExpiry(validUntil time.Time) time.Time {
	cap := time.Now().Add(qrMaxValidity)
	if validUntil.After(cap) {
		return cap
	}
	return validUntil
}

func (s *service) CreateQRCode(ctx context.Context, complexID uuid.UUID, createdBy uuid.UUID, req CreateQRCodeRequest) (*AccessGrantResponse, error) {
	if err := s.checkUnitOwnership(ctx, ActionCreateGrant, createdBy, complexID, req.UnitID); err != nil {
		return nil, err
	}
	visitorName := "Guest"
	if req.VisitorName != nil {
		visitorName = *req.VisitorName
	}
	createReq := CreateAccessGrantRequest{
		UnitID:      req.UnitID,
		Method:      MethodQR,
		VisitorType: VisitorTypeVisitor,
		VisitorName: visitorName,
		ValidFrom:   req.ValidFrom,
		ValidUntil:  clampQRExpiry(req.ValidUntil),
	}
	grant, err := s.repo.CreateAccessGrant(ctx, complexID, createReq, createdBy)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, &createdBy, &complexID, "access_grant", &grant.ID, ActionCreateGrant, audit.ResultAllowed, "")

	var qrCodeURL string
	if grant.QRCodeHash != nil {
		qrCodeURL, err = s.generateQRDataURL(*grant.QRCodeHash)
		if err != nil {
			s.logger.Error("Failed to generate QR code image", "error", err)
			// Don't fail the request, just return without image
		}
	}

	unitName, err := s.directory.GetUnitName(ctx, grant.UnitID)
	if err != nil {
		s.logger.Warn("Failed to get unit name", "error", err)
		unitName = "Unknown Unit"
	}

	creatorName := "Self"
	if name, err := s.directory.GetUserDisplayName(ctx, createdBy); err == nil && strings.TrimSpace(name) != "" {
		creatorName = name
	}

	s.notify(ctx, notifier.EventQRCreated, grant)

	return &AccessGrantResponse{
		AccessGrant: *grant,
		CreatorName: creatorName,
		UnitName:    unitName,
		QRCodeURL:   qrCodeURL,
	}, nil
}

func (s *service) generateQRDataURL(content string) (string, error) {
	q, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return "", err
	}
	q.BackgroundColor = color.White
	q.ForegroundColor = color.Black

	png, err := q.PNG(256)
	if err != nil {
		return "", err
	}

	encoded := base64.StdEncoding.EncodeToString(png)
	return fmt.Sprintf("data:image/png;base64,%s", encoded), nil
}

func (s *service) CreateQRBatch(ctx context.Context, complexID uuid.UUID, createdBy uuid.UUID, req CreateQRBatchRequest) ([]*AccessGrant, error) {
	if err := s.checkUnitOwnership(ctx, ActionCreateGrant, createdBy, complexID, req.UnitID); err != nil {
		return nil, err
	}
	grants := make([]*AccessGrant, 0, req.Quantity)
	clampedValidUntil := clampQRExpiry(req.ValidUntil)

	for i := 0; i < req.Quantity; i++ {
		createReq := CreateAccessGrantRequest{
			UnitID:      req.UnitID,
			Method:      MethodQR,
			VisitorType: VisitorTypeVisitor,
			ValidFrom:   &req.ValidFrom,
			ValidUntil:  clampedValidUntil,
		}
		grant, err := s.repo.CreateAccessGrant(ctx, complexID, createReq, createdBy)
		if err != nil {
			return nil, fmt.Errorf("failed to create QR code %d: %w", i+1, err)
		}
		grants = append(grants, grant)
	}

	if len(grants) > 0 {
		s.audit(ctx, &createdBy, &complexID, "access_grant", nil, ActionCreateGrant, audit.ResultAllowed, fmt.Sprintf("batch of %d", len(grants)))
		s.notify(ctx, notifier.EventQRCreated, grants[len(grants)-1])
	}

	return grants, nil
}

func (s *service) GetQRCodes(ctx context.Context, complexID uuid.UUID) ([]*AccessGrant, error) {
	method := MethodQR
	return s.repo.GetComplexAccessGrants(ctx, complexID, &method, nil)
}

func (s *service) CreatePreAuth(ctx context.Context, complexID uuid.UUID, createdBy uuid.UUID, req CreatePreAuthRequest) (*AccessGrant, error) {
	if err := s.checkUnitOwnership(ctx, ActionCreateGrant, createdBy, complexID, req.UnitID); err != nil {
		return nil, err
	}
	createReq := CreateAccessGrantRequest{
		UnitID:          req.UnitID,
		Method:          MethodPreAuth,
		VisitorType:     req.VisitorType,
		VisitorName:     req.VisitorName,
		VisitorIDNumber: &req.VisitorIDNumber,
		Details:         req.Details,
		ValidFrom:       &req.ValidFrom,
		ValidUntil:      req.ValidUntil,
	}
	grant, err := s.repo.CreateAccessGrant(ctx, complexID, createReq, createdBy)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, &createdBy, &complexID, "access_grant", &grant.ID, ActionCreateGrant, audit.ResultAllowed, "")
	s.notify(ctx, notifier.EventPreAuthCreated, grant)
	return grant, nil
}

func (s *service) GetPreAuthorizations(ctx context.Context, complexID uuid.UUID) ([]*AccessGrant, error) {
	method := MethodPreAuth
	return s.repo.GetComplexAccessGrants(ctx, complexID, &method, nil)
}

// IDOR guard #1: the body's unit_id must belong to the caller's complex.
// IDOR guard #2: the caller must be associated with that unit — complex scope
// alone is not enough, since any complex member could otherwise mint a grant
// into a neighbor's unit.
func (s *service) CreateAccessGrant(ctx context.Context, complexID uuid.UUID, createdBy uuid.UUID, req CreateAccessGrantRequest) (*AccessGrant, error) {
	if err := s.checkUnitOwnership(ctx, ActionCreateGrant, createdBy, complexID, req.UnitID); err != nil {
		return nil, err
	}
	if req.Method == MethodQR {
		req.ValidUntil = clampQRExpiry(req.ValidUntil)
	}
	grant, err := s.repo.CreateAccessGrant(ctx, complexID, req, createdBy)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, &createdBy, &complexID, "access_grant", &grant.ID, ActionCreateGrant, audit.ResultAllowed, "")
	s.notify(ctx, notifier.EventGrantCreated, grant)
	return grant, nil
}

// The caller must be in the grant's complex AND associated with its target
// unit: the payload carries the visitor's ID number (the PRE_AUTH entry
// credential) and the QR hash, so complex scope alone would let any member
// read a neighbor's grant by id.
func (s *service) GetAccessGrant(ctx context.Context, complexID uuid.UUID, userID uuid.UUID, id uuid.UUID) (*AccessGrant, error) {
	grant, err := s.repo.GetAccessGrant(ctx, id)
	if err != nil {
		return nil, err
	}
	if grant.ComplexID != complexID {
		s.audit(ctx, &userID, &complexID, "access_grant", &id, ActionGetGrant, audit.ResultDenied, "cross-tenant access attempt")
		return nil, ErrUnitAccessDenied
	}
	if err := s.assertUnitAccess(ctx, userID, grant.UnitID); err != nil {
		s.audit(ctx, &userID, &complexID, "access_grant", &id, ActionGetGrant, audit.ResultDenied, err.Error())
		return nil, err
	}
	return grant, nil
}

// The unit is read from the stored grant (never from the client) and the
// caller must be associated with it, otherwise any complex member could
// revoke a neighbor's access.
func (s *service) CancelAccessGrant(ctx context.Context, complexID uuid.UUID, userID uuid.UUID, id uuid.UUID) error {
	grant, err := s.repo.GetAccessGrant(ctx, id)
	if err != nil {
		return err
	}
	if grant.ComplexID != complexID {
		s.audit(ctx, &userID, &complexID, "access_grant", &id, ActionCancelGrant, audit.ResultDenied, "cross-tenant access attempt")
		return ErrUnitAccessDenied
	}
	if err := s.assertUnitAccess(ctx, userID, grant.UnitID); err != nil {
		s.audit(ctx, &userID, &complexID, "access_grant", &id, ActionCancelGrant, audit.ResultDenied, err.Error())
		return err
	}
	if err := s.repo.CancelAccessGrant(ctx, id); err != nil {
		return err
	}
	s.audit(ctx, &userID, &complexID, "access_grant", &id, ActionCancelGrant, audit.ResultAllowed, "")
	s.notify(ctx, notifier.EventGrantCancelled, grant)
	return nil
}

func (s *service) ValidateAccess(ctx context.Context, req ValidateAccessRequest) (*ValidateAccessResponse, error) {
	var actorID *uuid.UUID
	if id, err := authmw.GetUserIDAsUUID(ctx); err == nil {
		actorID = &id
	}

	complexID, err := authmw.GetComplexIDAsUUID(ctx)
	if err != nil {
		s.audit(ctx, actorID, nil, "access_grant", nil, ActionValidateGrant, audit.ResultDenied, err.Error())
		return nil, ErrComplexIDRequired
	}
	req.ComplexID = complexID

	deny := func(reason string) {
		s.audit(ctx, actorID, &complexID, "access_grant", nil, ActionValidateGrant, audit.ResultDenied, reason)
	}

	var grant *AccessGrant

	switch req.Method {
	case MethodQR:
		if req.QRCode == nil || *req.QRCode == "" {
			deny(ErrQRCodeRequired.Error())
			return nil, ErrQRCodeRequired
		}
		grant, err = s.repo.GetAccessGrantByQRHash(ctx, *req.QRCode)
		if err != nil {
			if errors.Is(err, ErrAccessGrantNotFound) {
				deny(err.Error())
				return &ValidateAccessResponse{Valid: false, Message: err.Error()}, nil
			}
			return nil, err
		}
		if grant.ComplexID != req.ComplexID {
			deny("QR belongs to a different complex")
			return &ValidateAccessResponse{
				Valid:   false,
				Message: "No active authorization found for this complex",
			}, nil
		}
	case MethodPreAuth:
		if req.VisitorIDNumber == nil || *req.VisitorIDNumber == "" {
			deny(ErrVisitorIDRequired.Error())
			return nil, ErrVisitorIDRequired
		}
		grants, err := s.repo.GetActiveAccessGrantsByVisitorID(ctx, req.ComplexID, *req.VisitorIDNumber)
		if err != nil {
			return nil, err
		}
		if len(grants) == 0 {
			deny("no active authorization for visitor ID")
			return &ValidateAccessResponse{
				Valid:   false,
				Message: "No active authorization found for this visitor",
			}, nil
		}
		grant = grants[0] // Use first active grant
	default:
		deny(fmt.Sprintf("unsupported method %q", req.Method))
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedMethod, req.Method)
	}

	now := time.Now()
	if grant.Status != StatusActive {
		s.audit(ctx, actorID, &complexID, "access_grant", &grant.ID, ActionValidateGrant, audit.ResultDenied, "status "+grant.Status)
		return &ValidateAccessResponse{
			Valid:   false,
			Message: fmt.Sprintf("Access grant is %s", grant.Status),
		}, nil
	}
	if grant.ValidFrom != nil && now.Before(*grant.ValidFrom) {
		s.audit(ctx, actorID, &complexID, "access_grant", &grant.ID, ActionValidateGrant, audit.ResultDenied, "not yet valid")
		return &ValidateAccessResponse{Valid: false, Message: "Access grant is not yet valid"}, nil
	}
	if grant.ValidUntil != nil && now.After(*grant.ValidUntil) {
		s.audit(ctx, actorID, &complexID, "access_grant", &grant.ID, ActionValidateGrant, audit.ResultDenied, "expired")
		return &ValidateAccessResponse{Valid: false, Message: "Access grant has expired"}, nil
	}

	// Consumption is the server's decision, not the client's: every grant is
	// single-use, consumed atomically on its first successful validation —
	// so a QR/PRE_AUTH scanned twice is accepted exactly once.
	if grant.SingleUse {
		var usedBy uuid.UUID
		if actorID != nil {
			usedBy = *actorID
		}
		won, err := s.repo.TryMarkGrantUsed(ctx, grant.ID, usedBy)
		if err != nil {
			return nil, fmt.Errorf("failed to mark grant as used: %w", err)
		}
		if !won {
			s.audit(ctx, actorID, &complexID, "access_grant", &grant.ID, ActionValidateGrant, audit.ResultDenied, "already used")
			return &ValidateAccessResponse{Valid: false, Message: "Access grant has already been used"}, nil
		}
	}

	s.audit(ctx, actorID, &complexID, "access_grant", &grant.ID, ActionValidateGrant, audit.ResultAllowed, "")
	s.notify(ctx, notifier.EventGrantValidated, grant)

	return &ValidateAccessResponse{
		Valid:       true,
		Message:     "Access granted",
		AccessGrant: grant,
		UnitID:      &grant.UnitID,
		VisitorName: &grant.VisitorName,
		VisitorType: &grant.VisitorType,
	}, nil
}

func (s *service) GetUnitAccessGrants(ctx context.Context, unitID uuid.UUID, status *string) ([]*AccessGrant, error) {
	return s.repo.GetUnitAccessGrants(ctx, unitID, status)
}
