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
	"controldeaccesos/internal/qrsystem"
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
	GetQRCodes(ctx context.Context, complexID, userID uuid.UUID) ([]*AccessGrant, error)
	GrantSummary(ctx context.Context, complexID uuid.UUID) (*GrantSummary, error)
	CreatePreAuth(ctx context.Context, complexID uuid.UUID, createdBy uuid.UUID, req CreatePreAuthRequest) (*AccessGrant, error)
	GetPreAuthorizations(ctx context.Context, complexID, userID uuid.UUID) ([]*AccessGrant, error)

	CreateAccessGrant(ctx context.Context, complexID uuid.UUID, createdBy uuid.UUID, req CreateAccessGrantRequest) (*AccessGrant, error)
	GetAccessGrant(ctx context.Context, complexID uuid.UUID, userID uuid.UUID, id uuid.UUID) (*AccessGrant, error)
	CancelAccessGrant(ctx context.Context, complexID uuid.UUID, userID uuid.UUID, id uuid.UUID) error
	ValidateAccess(ctx context.Context, req ValidateAccessRequest) (*ValidateAccessResponse, error)
	GetUnitAccessGrants(ctx context.Context, unitID uuid.UUID, status *string) ([]*AccessGrant, error)
	QRSystemStatus() qrsystem.Public
}

// QRSystem is the integration that issues/reads QR codes, behind a circuit
// breaker: when it is down these return an error matching qrsystem.ErrUnavailable
// immediately instead of blocking.
type QRSystem interface {
	Issue(ctx context.Context) (string, error)
	Scan(ctx context.Context) error
	Availability() qrsystem.Public
}

// UnitMembershipChecker verifies a caller is associated with a unit â€” the
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
	GetUnitMemberIDs(ctx context.Context, unitID uuid.UUID) ([]uuid.UUID, error)
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
	qr          QRSystem
	repo        Repository
	directory   DirectoryRepository
	unitChecker UnitMembershipChecker
	notifier    Notifier
	auditor     Auditor
	logger      *slog.Logger
}

func NewService(repo Repository, directory DirectoryRepository, unitChecker UnitMembershipChecker, notif Notifier, auditor Auditor, qr QRSystem, logger *slog.Logger) Service {
	return &service{
		qr:          qr,
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

// issueQR asks the QR system for a code. With no QR system wired it returns ""
// and the repository derives one locally.
func (s *service) issueQR(ctx context.Context) (string, error) {
	if s.qr == nil {
		return "", nil
	}
	return s.qr.Issue(ctx)
}

func (s *service) QRSystemStatus() qrsystem.Public {
	if s.qr == nil {
		return qrsystem.Public{Available: true, State: "CLOSED"}
	}
	return s.qr.Availability()
}

// Grant lifecycle moments that notify the residents of the target unit.
const (
	eventGrantCreated   = "grant_created"
	eventGrantCancelled = "grant_cancelled"
	eventGrantValidated = "grant_validated"
)

// notify tells the residents of the grant's unit what happened. The actor (who
// caused it, if a resident) is left out: they already know. It runs in the
// background â€” recipient lookup included â€” so it never delays or fails the
// request.
func (s *service) notify(ctx context.Context, event string, grant *AccessGrant, actor *uuid.UUID) {
	if s.notifier == nil || grant == nil {
		return
	}
	g := *grant
	bgCtx := context.WithoutCancel(ctx)

	go func() {
		members, err := s.directory.GetUnitMemberIDs(bgCtx, g.UnitID)
		if err != nil {
			s.logger.Warn("notify: failed to resolve unit members", "unit_id", g.UnitID, "error", err)
			return
		}
		recipients := make([]uuid.UUID, 0, len(members))
		for _, id := range members {
			if actor == nil || id != *actor {
				recipients = append(recipients, id)
			}
		}
		if len(recipients) == 0 {
			return
		}

		visitor := strings.TrimSpace(g.VisitorName)
		if visitor == "" {
			visitor = "Un visitante"
		}
		unit := "tu unidad"
		if name, err := s.directory.GetUnitName(bgCtx, g.UnitID); err == nil && strings.TrimSpace(name) != "" {
			unit = name
		}

		category, title, body := notifier.CategoryAccess, "", ""
		switch event {
		case eventGrantCreated:
			title, body = "Acceso autorizado", fmt.Sprintf("Se autorizÃ³ el ingreso de %s a %s", visitor, unit)
		case eventGrantCancelled:
			title, body = "Acceso cancelado", fmt.Sprintf("Se cancelÃ³ la autorizaciÃ³n de %s para %s", visitor, unit)
		case eventGrantValidated:
			category = notifier.CategoryVisitor
			title, body = "Visitante en porterÃ­a", fmt.Sprintf("%s ingresÃ³ a %s", visitor, unit)
		default:
			return
		}

		complexID := g.ComplexID
		s.notifier.Notify(bgCtx, notifier.Event{
			ComplexID: &complexID,
			UserIDs:   recipients,
			Category:  category,
			Title:     title,
			Body:      body,
			Data: map[string]any{
				"event":    event,
				"grant_id": g.ID.String(),
				"unit_id":  g.UnitID.String(),
				"method":   g.Method,
			},
		})
	}()
}

// audit writes a best-effort append-only trail entry. A nil auditor (e.g.
// unit tests) or a write failure never affects the caller â€” it's only
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
	code, err := s.issueQR(ctx)
	if err != nil {
		s.audit(ctx, &createdBy, &complexID, "access_grant", nil, ActionCreateGrant, audit.ResultDenied, "qr system unavailable")
		return nil, err
	}
	createReq := CreateAccessGrantRequest{
		UnitID:      req.UnitID,
		Method:      MethodQR,
		VisitorType: VisitorTypeVisitor,
		VisitorName: visitorName,
		ValidFrom:   req.ValidFrom,
		ValidUntil:  clampQRExpiry(req.ValidUntil),
		qrCode:      code,
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

	s.notify(ctx, eventGrantCreated, grant, &createdBy)

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
	clampedValidUntil := clampQRExpiry(req.ValidUntil)

	// 1) Obtain every code first: if the QR system fails midway nothing has been
	//    written yet. 2) Insert them all in ONE transaction: all or none.
	reqs := make([]CreateAccessGrantRequest, 0, req.Quantity)
	for i := 0; i < req.Quantity; i++ {
		code, err := s.issueQR(ctx)
		if err != nil {
			s.audit(ctx, &createdBy, &complexID, "access_grant", nil, ActionCreateGrant, audit.ResultDenied, "qr system unavailable")
			return nil, err
		}
		reqs = append(reqs, CreateAccessGrantRequest{
			UnitID:      req.UnitID,
			Method:      MethodQR,
			VisitorType: VisitorTypeVisitor,
			ValidFrom:   &req.ValidFrom,
			ValidUntil:  clampedValidUntil,
			qrCode:      code,
		})
	}
	grants, err := s.repo.CreateAccessGrantsTx(ctx, complexID, createdBy, reqs)
	if err != nil {
		return nil, fmt.Errorf("failed to create QR batch: %w", err)
	}

	if len(grants) > 0 {
		s.audit(ctx, &createdBy, &complexID, "access_grant", nil, ActionCreateGrant, audit.ResultAllowed, fmt.Sprintf("batch of %d", len(grants)))
		s.notify(ctx, eventGrantCreated, grants[len(grants)-1], &createdBy)
	}

	return grants, nil
}

// GetQRCodes lists the QR grants of the caller's own units only. A QR code is a
// bearer credential: listing the whole complex let any resident take a
// neighbour's pass. Staff get counts via GrantSummary, never the credentials.
func (s *service) GetQRCodes(ctx context.Context, complexID, userID uuid.UUID) ([]*AccessGrant, error) {
	return s.repo.GetComplexAccessGrantsForUser(ctx, complexID, userID, MethodQR)
}

func (s *service) GrantSummary(ctx context.Context, complexID uuid.UUID) (*GrantSummary, error) {
	return s.repo.GrantSummary(ctx, complexID)
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
	s.notify(ctx, eventGrantCreated, grant, &createdBy)
	return grant, nil
}

// GetPreAuthorizations: same rule as GetQRCodes (the visitor's ID number is the credential).
func (s *service) GetPreAuthorizations(ctx context.Context, complexID, userID uuid.UUID) ([]*AccessGrant, error) {
	return s.repo.GetComplexAccessGrantsForUser(ctx, complexID, userID, MethodPreAuth)
}

// IDOR guard #1: the body's unit_id must belong to the caller's complex.
// IDOR guard #2: the caller must be associated with that unit â€” complex scope
// alone is not enough, since any complex member could otherwise mint a grant
// into a neighbor's unit.
func (s *service) CreateAccessGrant(ctx context.Context, complexID uuid.UUID, createdBy uuid.UUID, req CreateAccessGrantRequest) (*AccessGrant, error) {
	if err := s.checkUnitOwnership(ctx, ActionCreateGrant, createdBy, complexID, req.UnitID); err != nil {
		return nil, err
	}
	if req.Method == MethodQR {
		req.ValidUntil = clampQRExpiry(req.ValidUntil)
		code, err := s.issueQR(ctx)
		if err != nil {
			s.audit(ctx, &createdBy, &complexID, "access_grant", nil, ActionCreateGrant, audit.ResultDenied, "qr system unavailable")
			return nil, err
		}
		req.qrCode = code
	}
	grant, err := s.repo.CreateAccessGrant(ctx, complexID, req, createdBy)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, &createdBy, &complexID, "access_grant", &grant.ID, ActionCreateGrant, audit.ResultAllowed, "")
	s.notify(ctx, eventGrantCreated, grant, &createdBy)
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
	s.notify(ctx, eventGrantCancelled, grant, &userID)
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
		// The QR reader at the gate is part of the QR system: if it doesn't
		// answer, fail fast so the guard can fall back to PRE_AUTH.
		if s.qr != nil {
			if err := s.qr.Scan(ctx); err != nil {
				deny("qr system unavailable")
				return nil, err
			}
		}
		grant, err = s.repo.GetAccessGrantByQRHash(ctx, *req.QRCode)
		if err != nil {
			if errors.Is(err, ErrAccessGrantNotFound) {
				deny(err.Error())
				return &ValidateAccessResponse{Valid: false, Message: err.Error(), Reason: ReasonNotFound}, nil
			}
			return nil, err
		}
		if grant.ComplexID != req.ComplexID {
			// The audit log keeps the real reason, but the caller must not learn
			// that this code exists elsewhere: answer exactly as for an unknown code.
			deny("QR belongs to a different complex")
			return &ValidateAccessResponse{
				Valid:   false,
				Message: ErrAccessGrantNotFound.Error(),
				Reason:  ReasonNotFound,
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
				Reason:  ReasonNoAuthorization,
			}, nil
		}
		// Prefer a grant whose window is open right now; if none is, report on the
		// newest one so the guard learns why (not started yet / expired).
		grant = grants[0]
		for _, g := range grants {
			if (g.ValidFrom == nil || !time.Now().Before(*g.ValidFrom)) && (g.ValidUntil == nil || !time.Now().After(*g.ValidUntil)) {
				grant = g
				break
			}
		}
	default:
		deny(fmt.Sprintf("unsupported method %q", req.Method))
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedMethod, req.Method)
	}

	now := time.Now()
	if grant.Status != StatusActive {
		s.audit(ctx, actorID, &complexID, "access_grant", &grant.ID, ActionValidateGrant, audit.ResultDenied, "status "+grant.Status)
		reason := ReasonExpired
		switch grant.Status {
		case StatusUsed:
			reason = ReasonAlreadyUsed
		case StatusCancelled:
			reason = ReasonCancelled
		}
		return &ValidateAccessResponse{
			Valid:   false,
			Message: fmt.Sprintf("Access grant is %s", grant.Status),
			Reason:  reason,
		}, nil
	}
	if grant.ValidFrom != nil && now.Before(*grant.ValidFrom) {
		s.audit(ctx, actorID, &complexID, "access_grant", &grant.ID, ActionValidateGrant, audit.ResultDenied, "not yet valid")
		return &ValidateAccessResponse{
			Valid: false, Message: "Access grant is not yet valid", Reason: ReasonNotYetValid,
			ValidFrom: grant.ValidFrom, ValidUntil: grant.ValidUntil,
		}, nil
	}
	if grant.ValidUntil != nil && now.After(*grant.ValidUntil) {
		s.audit(ctx, actorID, &complexID, "access_grant", &grant.ID, ActionValidateGrant, audit.ResultDenied, "expired")
		return &ValidateAccessResponse{
			Valid: false, Message: "Access grant has expired", Reason: ReasonExpired,
			ValidFrom: grant.ValidFrom, ValidUntil: grant.ValidUntil,
		}, nil
	}

	// Consumption is the server's decision, not the client's: every grant is
	// single-use, consumed atomically on its first successful validation â€”
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
			return &ValidateAccessResponse{Valid: false, Message: "Access grant has already been used", Reason: ReasonAlreadyUsed}, nil
		}
	}

	s.audit(ctx, actorID, &complexID, "access_grant", &grant.ID, ActionValidateGrant, audit.ResultAllowed, "")
	s.notify(ctx, eventGrantValidated, grant, nil)

	// The gate shows where the visitor is going; a failed lookup only omits it.
	var unitName *string
	if name, err := s.directory.GetUnitName(ctx, grant.UnitID); err == nil {
		unitName = &name
	} else {
		s.logger.Warn("Failed to get unit name", "error", err)
	}

	return &ValidateAccessResponse{
		Valid:       true,
		Message:     "Access granted",
		AccessGrant: grant,
		UnitID:      &grant.UnitID,
		UnitName:    unitName,
		VisitorName: &grant.VisitorName,
		VisitorType: &grant.VisitorType,
	}, nil
}

func (s *service) GetUnitAccessGrants(ctx context.Context, unitID uuid.UUID, status *string) ([]*AccessGrant, error) {
	return s.repo.GetUnitAccessGrants(ctx, unitID, status)
}
