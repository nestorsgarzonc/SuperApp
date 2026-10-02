package repository

import (
	"context"
	"superapp/auth/internal/core/domain"
	"time"
)

type SessionRepository interface {
	CreateSession(context.Context, string, string, string, time.Time, string, string, string) error
	RotateRefresh(context.Context, string, string, string, time.Time, time.Time) (sessionID, userID, email string, err error)
	RevokeRefresh(context.Context, string) error
	RevokeAllSessions(context.Context, string) error
	RevokeSession(context.Context, string, string) error
	Sessions(context.Context, string) ([]domain.Session, error)
}
