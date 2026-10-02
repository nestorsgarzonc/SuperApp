package application

import (
	"context"
	"time"

	"superapp/control-de-accesos/internal/domain"
)

type Repository interface {
	Ping(context.Context) error
	Actor(ctx context.Context, complexID, userID string) (domain.Actor, error)
	UnitInComplex(ctx context.Context, complexID, unitID string) (bool, error)
	UserInUnit(ctx context.Context, userID, unitID string) (bool, error)

	CreatePass(context.Context, *domain.Pass) error
	GetPass(ctx context.Context, complexID, id string) (*domain.Pass, error)
	ListPasses(ctx context.Context, complexID, unitID string, limit int) ([]domain.Pass, error)
	PassByToken(ctx context.Context, complexID, token string) (*domain.Pass, error)
	MarkUsed(ctx context.Context, complexID, id, guardID string, now time.Time) (*domain.Pass, error)
	RevokePass(ctx context.Context, complexID, id string) (*domain.Pass, error)
	ExtendPass(ctx context.Context, complexID, id string, validUntil, now time.Time) (*domain.Pass, error)
	MarkCheckedOut(ctx context.Context, complexID, id, guardID string, now time.Time) (*domain.Pass, error)
	SweepExpired(context.Context, time.Time) error
	Checkpoint(ctx context.Context, complexID string, limit int, now time.Time) (*domain.CheckpointFeed, error)
	Notify(ctx context.Context, complexID, userID, title, body, kind string) error

	CreatePreauth(context.Context, *domain.Preauth) error
	ListPreauths(ctx context.Context, complexID, hostID string) ([]domain.Preauth, error)
	RevokePreauth(ctx context.Context, complexID, hostID, id string) error
	LookupPreauth(ctx context.Context, complexID, plate string, now time.Time) (*domain.PreauthMatch, error)
	LogAccess(context.Context, *domain.AccessLog) error
	ListAccess(ctx context.Context, complexID string) ([]domain.AccessLog, error)

	Complexes(context.Context) ([]domain.Complex, error)
	People(context.Context) ([]domain.Person, error)
	Notifications(ctx context.Context, complexID, userID string) ([]domain.Notification, error)
}
