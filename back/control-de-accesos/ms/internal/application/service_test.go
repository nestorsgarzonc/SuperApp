package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"superapp/control-de-accesos/internal/domain"
)

type fakeRepo struct {
	inComplex bool
	member    bool
	created   *domain.Pass
	preauth   *domain.Preauth
}

func (f *fakeRepo) Ping(context.Context) error { return nil }
func (f *fakeRepo) Actor(context.Context, string, string) (domain.Actor, error) {
	return domain.Actor{}, domain.ErrUnauthorized
}
func (f *fakeRepo) UnitInComplex(context.Context, string, string) (bool, error) {
	return f.inComplex, nil
}
func (f *fakeRepo) UserInUnit(context.Context, string, string) (bool, error) { return f.member, nil }
func (f *fakeRepo) CreatePass(_ context.Context, p *domain.Pass) error {
	p.ID = "pass-1"
	p.Status = domain.StatusActive
	now := time.Now()
	p.CreatedAt, p.UpdatedAt = now, now
	f.created = p
	return nil
}
func (f *fakeRepo) GetPass(context.Context, string, string) (*domain.Pass, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeRepo) ListPasses(context.Context, string, string, int) ([]domain.Pass, error) {
	return nil, nil
}
func (f *fakeRepo) PassByToken(context.Context, string, string) (*domain.Pass, error) {
	return nil, domain.ErrInvalidToken
}
func (f *fakeRepo) MarkUsed(context.Context, string, string, string, time.Time) (*domain.Pass, error) {
	return nil, domain.ErrPassNotActive
}
func (f *fakeRepo) RevokePass(context.Context, string, string) (*domain.Pass, error) {
	return nil, domain.ErrPassNotActive
}
func (f *fakeRepo) ExtendPass(context.Context, string, string, time.Time, time.Time) (*domain.Pass, error) {
	return nil, domain.ErrPassNotExtendable
}
func (f *fakeRepo) MarkCheckedOut(context.Context, string, string, string, time.Time) (*domain.Pass, error) {
	return nil, domain.ErrPassNotCheckedIn
}
func (f *fakeRepo) SweepExpired(context.Context, time.Time) error { return nil }
func (f *fakeRepo) Checkpoint(context.Context, string, int, time.Time) (*domain.CheckpointFeed, error) {
	return nil, nil
}
func (f *fakeRepo) Notify(context.Context, string, string, string, string, string) error { return nil }
func (f *fakeRepo) CreatePreauth(_ context.Context, p *domain.Preauth) error {
	p.ID = "pre-1"
	p.Status = domain.StatusActive
	p.CreatedAt = time.Now()
	f.preauth = p
	return nil
}
func (f *fakeRepo) ListPreauths(context.Context, string, string) ([]domain.Preauth, error) {
	return nil, nil
}
func (f *fakeRepo) RevokePreauth(context.Context, string, string, string) error {
	return domain.ErrPreauthNotFound
}
func (f *fakeRepo) LookupPreauth(context.Context, string, string, time.Time) (*domain.PreauthMatch, error) {
	return nil, nil
}
func (f *fakeRepo) LogAccess(context.Context, *domain.AccessLog) error { return nil }
func (f *fakeRepo) ListAccess(context.Context, string) ([]domain.AccessLog, error) {
	return nil, nil
}
func (f *fakeRepo) Complexes(context.Context) ([]domain.Complex, error) { return nil, nil }
func (f *fakeRepo) People(context.Context) ([]domain.Person, error)     { return nil, nil }
func (f *fakeRepo) Notifications(context.Context, string, string) ([]domain.Notification, error) {
	return nil, nil
}

func TestCreatePass_RejectsNeighborUnit(t *testing.T) {
	repo := &fakeRepo{inComplex: true, member: false}
	svc := New(repo)
	_, err := svc.CreatePass(context.Background(), domain.Actor{ComplexID: "c", UserID: "u"}, domain.CreatePass{
		UnitID: "unit-b", VisitorName: "Maria Visitor", ValidUntil: time.Now().Add(2 * time.Hour),
	})
	if !errors.Is(err, domain.ErrUnitAccessDenied) {
		t.Fatalf("got %v", err)
	}
	if repo.created != nil {
		t.Fatal("neighbor pass was stored")
	}
}

func TestCreatePass_OwnUnitKeepsToken(t *testing.T) {
	repo := &fakeRepo{inComplex: true, member: true}
	svc := New(repo)
	svc.genToken = func() (string, error) { return "fixed-token", nil }
	pass, err := svc.CreatePass(context.Background(), domain.Actor{ComplexID: "c", UserID: "u"}, domain.CreatePass{
		UnitID: "unit-a", VisitorName: "  Maria Visitor ", ValidUntil: time.Now().Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if pass.Token != "fixed-token" || pass.VisitorName != "Maria Visitor" {
		t.Fatalf("%+v", pass)
	}
}

func TestCreatePreauth_RejectsNeighborUnit(t *testing.T) {
	repo := &fakeRepo{inComplex: true, member: false}
	svc := New(repo)
	_, err := svc.CreatePreauth(context.Background(), domain.Actor{ComplexID: "c", UserID: "u"}, domain.CreatePreauth{
		UnitID: "unit-b", Plate: "ABC-123", VisitorName: "Rappi Driver", ValidUntil: time.Now().Add(24 * time.Hour),
	})
	if !errors.Is(err, domain.ErrUnitAccessDenied) {
		t.Fatalf("got %v", err)
	}
}

func TestNormalizePlate(t *testing.T) {
	if got := domain.NormalizePlate(" abc-123 "); got != "ABC123" {
		t.Fatalf("got %q", got)
	}
}
