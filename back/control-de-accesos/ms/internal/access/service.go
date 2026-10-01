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

	"controldeaccesos/internal/notifier"
	authmw "controldeaccesos/internal/platform/middleware"
)

// Service defines business logic for access control operations.
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

// DirectoryRepository resolves the handful of unit/user facts the access
// feature needs (unit ownership, display names) from this service's own
// local directory read-model.
type DirectoryRepository interface {
	GetUnitComplexID(ctx context.Context, unitID uuid.UUID) (uuid.UUID, error)
	GetUnitName(ctx context.Context, unitID uuid.UUID) (string, error)
	GetUserDisplayName(ctx context.Context, userID uuid.UUID) (string, error)
}

// Notifier fires access events at the (future) notificaciones microservice.
// Best-effort: implementations must never block or fail the caller.
type Notifier interface {
	Notify(ctx context.Context, event notifier.Event)
}

type service struct {
	repo        Repository
	directory   DirectoryRepository
	unitChecker UnitMembershipChecker
	notifier    Notifier
	logger      *slog.Logger
}

func NewService(repo Repository, directory DirectoryRepository, unitChecker UnitMembershipChecker, notif Notifier, logger *slog.Logger) Service {
	return &service{
		repo:        repo,
		directory:   directory,
		unitChecker: unitChecker,
		notifier:    notif,
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

// notify fires a best-effort access event at the notificaciones microservice.
// No-op when s.notifier is nil (e.g. unit tests) or NOTIFICATIONS_URL is unset.
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

func (s *service) validateUnitBelongsToComplex(ctx context.Context, complexID, unitID uuid.UUID) error {
	unitComplexID, err := s.directory.GetUnitComplexID(ctx, unitID)
	if err != nil {
		return fmt.Errorf("failed to load unit: %w", err)
	}
	if unitComplexID != complexID {
		return fmt.Errorf("unit does not belong to the selected complex")
	}
	return nil
}

// CreateQRCode creates a single QR code access grant.
func (s *service) CreateQRCode(ctx context.Context, complexID uuid.UUID, createdBy uuid.UUID, req CreateQRCodeRequest) (*AccessGrantResponse, error) {
	if err := s.validateUnitBelongsToComplex(ctx, complexID, req.UnitID); err != nil {
		return nil, err
	}
	if err := s.assertUnitAccess(ctx, createdBy, req.UnitID); err != nil {
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
		ValidUntil:  req.ValidUntil,
	}
	grant, err := s.repo.CreateAccessGrant(ctx, complexID, createReq, createdBy)
	if err != nil {
		return nil, err
	}

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

// generateQRDataURL generates a Base64 encoded PNG data URL for the QR code.
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

// CreateQRBatch creates multiple QR codes at once.
func (s *service) CreateQRBatch(ctx context.Context, complexID uuid.UUID, createdBy uuid.UUID, req CreateQRBatchRequest) ([]*AccessGrant, error) {
	if err := s.validateUnitBelongsToComplex(ctx, complexID, req.UnitID); err != nil {
		return nil, err
	}
	if err := s.assertUnitAccess(ctx, createdBy, req.UnitID); err != nil {
		return nil, err
	}
	grants := make([]*AccessGrant, 0, req.Quantity)

	for i := 0; i < req.Quantity; i++ {
		createReq := CreateAccessGrantRequest{
			UnitID:      req.UnitID,
			Method:      MethodQR,
			VisitorType: VisitorTypeVisitor,
			ValidFrom:   &req.ValidFrom,
			ValidUntil:  req.ValidUntil,
		}
		grant, err := s.repo.CreateAccessGrant(ctx, complexID, createReq, createdBy)
		if err != nil {
			return nil, fmt.Errorf("failed to create QR code %d: %w", i+1, err)
		}
		grants = append(grants, grant)
	}

	if len(grants) > 0 {
		s.notify(ctx, notifier.EventQRCreated, grants[len(grants)-1])
	}

	return grants, nil
}

// GetQRCodes retrieves all QR code grants for a complex.
func (s *service) GetQRCodes(ctx context.Context, complexID uuid.UUID) ([]*AccessGrant, error) {
	method := MethodQR
	return s.repo.GetComplexAccessGrants(ctx, complexID, &method, nil)
}

// CreatePreAuth creates a pre-authorization access grant.
func (s *service) CreatePreAuth(ctx context.Context, complexID uuid.UUID, createdBy uuid.UUID, req CreatePreAuthRequest) (*AccessGrant, error) {
	if err := s.validateUnitBelongsToComplex(ctx, complexID, req.UnitID); err != nil {
		return nil, err
	}
	if err := s.assertUnitAccess(ctx, createdBy, req.UnitID); err != nil {
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
	s.notify(ctx, notifier.EventPreAuthCreated, grant)
	return grant, nil
}

// GetPreAuthorizations retrieves all pre-auth grants for a complex.
func (s *service) GetPreAuthorizations(ctx context.Context, complexID uuid.UUID) ([]*AccessGrant, error) {
	method := MethodPreAuth
	return s.repo.GetComplexAccessGrants(ctx, complexID, &method, nil)
}

// CreateAccessGrant creates a generic access grant (supports both QR and PRE_AUTH).
// IDOR guard #1: the body's unit_id must belong to the caller's complex.
// IDOR guard #2: the caller must be associated with that unit — complex scope
// alone is not enough, since any complex member could otherwise mint a grant
// into a neighbor's unit.
func (s *service) CreateAccessGrant(ctx context.Context, complexID uuid.UUID, createdBy uuid.UUID, req CreateAccessGrantRequest) (*AccessGrant, error) {
	if err := s.validateUnitBelongsToComplex(ctx, complexID, req.UnitID); err != nil {
		return nil, err
	}
	if err := s.assertUnitAccess(ctx, createdBy, req.UnitID); err != nil {
		return nil, err
	}
	grant, err := s.repo.CreateAccessGrant(ctx, complexID, req, createdBy)
	if err != nil {
		return nil, err
	}
	s.notify(ctx, notifier.EventGrantCreated, grant)
	return grant, nil
}

// GetAccessGrant retrieves an access grant by ID. The caller must be in the
// grant's complex AND associated with its target unit: the payload carries the
// visitor's ID number (the PRE_AUTH entry credential) and the QR hash, so
// complex scope alone would let any member read a neighbor's grant by id.
func (s *service) GetAccessGrant(ctx context.Context, complexID uuid.UUID, userID uuid.UUID, id uuid.UUID) (*AccessGrant, error) {
	grant, err := s.repo.GetAccessGrant(ctx, id)
	if err != nil {
		return nil, err
	}
	if grant.ComplexID != complexID {
		return nil, ErrUnitAccessDenied
	}
	if err := s.assertUnitAccess(ctx, userID, grant.UnitID); err != nil {
		return nil, err
	}
	return grant, nil
}

// CancelAccessGrant cancels an access grant. The unit is read from the stored
// grant (never from the client) and the caller must be associated with it,
// otherwise any complex member could revoke a neighbor's access.
func (s *service) CancelAccessGrant(ctx context.Context, complexID uuid.UUID, userID uuid.UUID, id uuid.UUID) error {
	grant, err := s.repo.GetAccessGrant(ctx, id)
	if err != nil {
		return err
	}
	if grant.ComplexID != complexID {
		return ErrUnitAccessDenied
	}
	if err := s.assertUnitAccess(ctx, userID, grant.UnitID); err != nil {
		return err
	}
	if err := s.repo.CancelAccessGrant(ctx, id); err != nil {
		return err
	}
	s.notify(ctx, notifier.EventGrantCancelled, grant)
	return nil
}

// ValidateAccess validates a QR code or PRE_AUTH access request.
func (s *service) ValidateAccess(ctx context.Context, req ValidateAccessRequest) (*ValidateAccessResponse, error) {
	complexID, err := authmw.GetComplexIDAsUUID(ctx)
	if err != nil {
		return nil, fmt.Errorf("complex ID is required")
	}
	req.ComplexID = complexID

	var grant *AccessGrant

	switch req.Method {
	case MethodQR:
		if req.QRCode == nil || *req.QRCode == "" {
			return nil, fmt.Errorf("QR code is required for QR validation")
		}
		grant, err = s.repo.GetAccessGrantByQRHash(ctx, *req.QRCode)
		if err != nil {
			if errors.Is(err, ErrAccessGrantNotFound) {
				return &ValidateAccessResponse{Valid: false, Message: err.Error()}, nil
			}
			return nil, err
		}
		if grant.ComplexID != req.ComplexID {
			return &ValidateAccessResponse{
				Valid:   false,
				Message: "No active authorization found for this complex",
			}, nil
		}
	case MethodPreAuth:
		if req.VisitorIDNumber == nil || *req.VisitorIDNumber == "" {
			return nil, fmt.Errorf("visitor ID number is required for PRE_AUTH validation")
		}
		grants, err := s.repo.GetActiveAccessGrantsByVisitorID(ctx, req.ComplexID, *req.VisitorIDNumber)
		if err != nil {
			return nil, err
		}
		if len(grants) == 0 {
			return &ValidateAccessResponse{
				Valid:   false,
				Message: "No active authorization found for this visitor",
			}, nil
		}
		grant = grants[0] // Use first active grant
	default:
		return nil, fmt.Errorf("unsupported validation method: %s", req.Method)
	}

	now := time.Now()
	if grant.Status != StatusActive {
		return &ValidateAccessResponse{
			Valid:   false,
			Message: fmt.Sprintf("Access grant is %s", grant.Status),
		}, nil
	}
	if grant.ValidFrom != nil && now.Before(*grant.ValidFrom) {
		return &ValidateAccessResponse{Valid: false, Message: "Access grant is not yet valid"}, nil
	}
	if grant.ValidUntil != nil && now.After(*grant.ValidUntil) {
		return &ValidateAccessResponse{Valid: false, Message: "Access grant has expired"}, nil
	}

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

// GetUnitAccessGrants retrieves access grants for a specific unit.
func (s *service) GetUnitAccessGrants(ctx context.Context, unitID uuid.UUID, status *string) ([]*AccessGrant, error) {
	return s.repo.GetUnitAccessGrants(ctx, unitID, status)
}
