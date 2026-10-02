package application

import (
	"time"

	"superapp/auth/internal/ports/repository"
)

type TokenClaims struct {
	Subject, Email, SessionID string
	ExpiresAt                 time.Time
}
type TokenIssuer interface {
	Issue(userID, email, sessionID string) (string, time.Time, error)
	Verify(token string) (*TokenClaims, error)
	JWKS() map[string]any
}
type PasswordHasher interface {
	Hash(string) (string, error)
	Verify(hash, password string) error
	Burn(password string)
}

type Repository = repository.Repository

type IDGenerator interface{ New() string }
