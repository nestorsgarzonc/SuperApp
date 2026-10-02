package repository

import (
	"context"
	"superapp/auth/internal/core/domain"
	"time"
)

type SessionRepository interface {
	CreateSession(context.Context, CreateSessionParams) error
	RotateRefresh(context.Context, RotateRefreshParams) (RotateRefreshResult, error)
	RevokeRefresh(context.Context, string) error
	RevokeAllSessions(context.Context, string) error
	RevokeSession(context.Context, RevokeSessionParams) error
	Sessions(context.Context, string) ([]domain.Session, error)
}

type CreateSessionParams struct {
	SessionID   string
	UserID      string
	RefreshHash string
	ExpiresAt   time.Time
	Device      string
	UserAgent   string
	IP          string
}

type RotateRefreshParams struct {
	OldHash    string
	NewHash    string
	NewTokenID string
	ExpiresAt  time.Time
	Now        time.Time
}

type RotateRefreshResult struct {
	SessionID string
	UserID    string
	Email     string
}

type RevokeSessionParams struct {
	UserID    string
	SessionID string
}
