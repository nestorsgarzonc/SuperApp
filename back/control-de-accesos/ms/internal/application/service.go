package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"superapp/control-de-accesos/internal/domain"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Service struct {
	repo     Repository
	now      func() time.Time
	genToken func() (string, error)
	log      *slog.Logger
}

func New(repo Repository) *Service {
	return &Service{
		repo:     repo,
		now:      time.Now,
		genToken: randomToken,
		log:      slog.Default(),
	}
}

func (s *Service) Ready(ctx context.Context) error { return s.repo.Ping(ctx) }

func (s *Service) Actor(ctx context.Context, complexID, userID string) (domain.Actor, error) {
	if !uuidPattern.MatchString(complexID) || !uuidPattern.MatchString(userID) {
		return domain.Actor{}, domain.ErrUnauthorized
	}
	return s.repo.Actor(ctx, complexID, userID)
}

func (s *Service) Context(ctx context.Context) ([]domain.Complex, []domain.Person, error) {
	complexes, err := s.repo.Complexes(ctx)
	if err != nil {
		return nil, nil, err
	}
	people, err := s.repo.People(ctx)
	if err != nil {
		return nil, nil, err
	}
	return complexes, people, nil
}

func (s *Service) Notifications(ctx context.Context, actor domain.Actor) ([]domain.Notification, error) {
	return s.repo.Notifications(ctx, actor.ComplexID, actor.UserID)
}

func (s *Service) CreatePass(ctx context.Context, actor domain.Actor, req domain.CreatePass) (*domain.Pass, error) {
	name := strings.TrimSpace(req.VisitorName)
	if name == "" {
		return nil, domain.ErrVisitorNameRequired
	}
	if req.UnitID == "" {
		return nil, domain.ErrUnitNotInComplex
	}
	now := s.now()
	validFrom := req.ValidFrom
	if validFrom.IsZero() {
		validFrom = now
	}
	if !req.ValidUntil.After(validFrom) || req.ValidUntil.Sub(validFrom) > domain.MaxValidity {
		return nil, domain.ErrInvalidWindow
	}
	if err := s.ownUnit(ctx, actor, req.UnitID); err != nil {
		return nil, err
	}
	token, err := s.genToken()
	if err != nil {
		return nil, err
	}
	pass := &domain.Pass{
		ComplexID: actor.ComplexID, UnitID: req.UnitID, HostUserID: actor.UserID,
		VisitorName: name, Document: strings.TrimSpace(req.Document), Token: token,
		ValidFrom: validFrom, ValidUntil: req.ValidUntil,
	}
	if err := s.repo.CreatePass(ctx, pass); err != nil {
		return nil, err
	}
	return pass, nil
}

func (s *Service) GetPass(ctx context.Context, actor domain.Actor, id string) (*domain.Pass, error) {
	pass, err := s.repo.GetPass(ctx, actor.ComplexID, id)
	if err != nil {
		return nil, err
	}
	if err := s.member(ctx, actor.UserID, pass.UnitID); err != nil {
		return nil, err
	}
	pass.Status = pass.EffectiveStatus(s.now())
	return pass, nil
}

func (s *Service) ListPasses(ctx context.Context, actor domain.Actor, unitID string, limit int) ([]domain.Pass, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if err := s.member(ctx, actor.UserID, unitID); err != nil {
		return nil, err
	}
	now := s.now()
	if err := s.repo.SweepExpired(ctx, now); err != nil {
		s.log.Warn("sweep expired passes", "error", err)
	}
	list, err := s.repo.ListPasses(ctx, actor.ComplexID, unitID, limit)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Status = list[i].EffectiveStatus(now)
	}
	return list, nil
}

func (s *Service) Checkpoint(ctx context.Context, actor domain.Actor, limit int) (*domain.CheckpointFeed, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	return s.repo.Checkpoint(ctx, actor.ComplexID, limit, s.now())
}

func (s *Service) RevokePass(ctx context.Context, actor domain.Actor, id string) (*domain.Pass, error) {
	existing, err := s.repo.GetPass(ctx, actor.ComplexID, id)
	if err != nil {
		return nil, err
	}
	if err := s.member(ctx, actor.UserID, existing.UnitID); err != nil {
		return nil, err
	}
	return s.repo.RevokePass(ctx, actor.ComplexID, id)
}

func (s *Service) ExtendPass(ctx context.Context, actor domain.Actor, id string, until time.Time) (*domain.Pass, error) {
	existing, err := s.repo.GetPass(ctx, actor.ComplexID, id)
	if err != nil {
		return nil, err
	}
	if err := s.member(ctx, actor.UserID, existing.UnitID); err != nil {
		return nil, err
	}
	now := s.now()
	if existing.Status != domain.StatusActive || !now.Before(existing.ValidUntil) {
		return nil, domain.ErrPassNotExtendable
	}
	if !until.After(existing.ValidUntil) || !until.After(now) || until.Sub(existing.ValidFrom) > domain.MaxValidity {
		return nil, domain.ErrInvalidExtension
	}
	return s.repo.ExtendPass(ctx, actor.ComplexID, id, until, now)
}

func (s *Service) ValidateToken(ctx context.Context, actor domain.Actor, token string) (*domain.Pass, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, domain.ErrInvalidToken
	}
	now := s.now()
	pass, err := s.repo.PassByToken(ctx, actor.ComplexID, token)
	if err != nil {
		return nil, err
	}
	switch pass.Status {
	case domain.StatusUsed, domain.StatusRevoked, domain.StatusCheckedOut:
		return nil, domain.ErrPassNotActive
	case domain.StatusExpired:
		return nil, domain.ErrPassExpired
	}
	if now.Before(pass.ValidFrom) {
		return nil, domain.ErrPassNotYetValid
	}
	if !now.Before(pass.ValidUntil) {
		return nil, domain.ErrPassExpired
	}
	used, err := s.repo.MarkUsed(ctx, actor.ComplexID, pass.ID, actor.UserID, now)
	if err != nil {
		return nil, err
	}
	s.notify(ctx, used, "Your guest has arrived", used.VisitorName+" has arrived and was admitted at the entrance.")
	stripped := used.WithoutToken()
	return &stripped, nil
}

func (s *Service) CheckOut(ctx context.Context, actor domain.Actor, token string) (*domain.Pass, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, domain.ErrInvalidToken
	}
	pass, err := s.repo.PassByToken(ctx, actor.ComplexID, token)
	if err != nil {
		return nil, err
	}
	if pass.Status != domain.StatusUsed {
		return nil, domain.ErrPassNotCheckedIn
	}
	out, err := s.repo.MarkCheckedOut(ctx, actor.ComplexID, pass.ID, actor.UserID, s.now())
	if err != nil {
		return nil, err
	}
	s.notify(ctx, out, "Your guest has left", out.VisitorName+" has left the premises.")
	stripped := out.WithoutToken()
	return &stripped, nil
}

func (s *Service) CreatePreauth(ctx context.Context, actor domain.Actor, req domain.CreatePreauth) (*domain.Preauth, error) {
	plate := domain.NormalizePlate(req.Plate)
	if plate == "" {
		return nil, domain.ErrInvalidPlate
	}
	name := strings.TrimSpace(req.VisitorName)
	if name == "" {
		return nil, domain.ErrVisitorNameRequired
	}
	if req.UnitID == "" {
		return nil, domain.ErrUnitNotInComplex
	}
	now := s.now()
	validFrom := req.ValidFrom
	if validFrom.IsZero() {
		validFrom = now
	}
	if !req.ValidUntil.After(validFrom) || !req.ValidUntil.After(now) {
		return nil, domain.ErrInvalidWindow
	}
	if err := s.ownUnit(ctx, actor, req.UnitID); err != nil {
		return nil, err
	}
	item := &domain.Preauth{
		ComplexID: actor.ComplexID, UnitID: req.UnitID, HostUserID: actor.UserID,
		Plate: plate, VisitorName: name, ValidFrom: validFrom, ValidUntil: req.ValidUntil,
	}
	if err := s.repo.CreatePreauth(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) ListPreauths(ctx context.Context, actor domain.Actor) ([]domain.Preauth, error) {
	return s.repo.ListPreauths(ctx, actor.ComplexID, actor.UserID)
}

func (s *Service) RevokePreauth(ctx context.Context, actor domain.Actor, id string) error {
	return s.repo.RevokePreauth(ctx, actor.ComplexID, actor.UserID, id)
}

func (s *Service) LookupPlate(ctx context.Context, actor domain.Actor, raw string) (*domain.Lookup, error) {
	plate := domain.NormalizePlate(raw)
	if plate == "" {
		return nil, domain.ErrInvalidPlate
	}
	match, err := s.repo.LookupPreauth(ctx, actor.ComplexID, plate, s.now())
	if err != nil {
		return nil, err
	}
	return &domain.Lookup{Plate: plate, Found: false, VisitorPreauth: match}, nil
}

func (s *Service) LogAccess(ctx context.Context, actor domain.Actor, plateRaw, directionRaw, note string) (*domain.AccessLog, error) {
	plate := domain.NormalizePlate(plateRaw)
	if plate == "" {
		return nil, domain.ErrInvalidPlate
	}
	direction, err := domain.NormalizeDirection(directionRaw)
	if err != nil {
		return nil, err
	}
	match, err := s.repo.LookupPreauth(ctx, actor.ComplexID, plate, s.now())
	if err != nil {
		return nil, err
	}
	entry := &domain.AccessLog{
		ComplexID: actor.ComplexID, Plate: plate, Direction: direction,
		GuardID: actor.UserID, Note: strings.TrimSpace(note),
	}
	if match != nil {
		id := match.ID
		entry.PreauthID = &id
	}
	if err := s.repo.LogAccess(ctx, entry); err != nil {
		return nil, err
	}
	entry.VisitorPreauth = match
	return entry, nil
}

func (s *Service) ListAccess(ctx context.Context, actor domain.Actor) ([]domain.AccessLog, error) {
	return s.repo.ListAccess(ctx, actor.ComplexID)
}

func (s *Service) ownUnit(ctx context.Context, actor domain.Actor, unitID string) error {
	ok, err := s.repo.UnitInComplex(ctx, actor.ComplexID, unitID)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrUnitNotInComplex
	}
	return s.member(ctx, actor.UserID, unitID)
}

func (s *Service) member(ctx context.Context, userID, unitID string) error {
	ok, err := s.repo.UserInUnit(ctx, userID, unitID)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrUnitAccessDenied
	}
	return nil
}

func (s *Service) notify(ctx context.Context, pass *domain.Pass, title, body string) {
	if err := s.repo.Notify(ctx, pass.ComplexID, pass.HostUserID, title, body, "VISITOR_PASS"); err != nil {
		s.log.Error("notify host", "error", err, "pass_id", pass.ID)
	}
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
