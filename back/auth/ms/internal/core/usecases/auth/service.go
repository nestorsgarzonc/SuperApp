package auth

import (
	"time"

	"superapp/auth/internal/core/ports/repository"
	"superapp/auth/internal/core/ports/security"
	"superapp/auth/internal/core/usecases/contracts"
)

type Service struct {
	users     repository.UserRepository
	sessions  repository.SessionRepository
	passwords security.PasswordHasher
	tokens    security.TokenIssuer
	newID     security.IDGenerator
	cfg       contracts.Config
	now       func() time.Time
}

func New(users repository.UserRepository, sessions repository.SessionRepository, passwords security.PasswordHasher, tokens security.TokenIssuer, newID security.IDGenerator, cfg contracts.Config) *Service {
	return &Service{users: users, sessions: sessions, passwords: passwords, tokens: tokens, newID: newID, cfg: cfg, now: time.Now}
}
func (s *Service) Authenticate(raw string) (*security.TokenClaims, error) {
	return s.tokens.Verify(raw)
}
func (s *Service) JWKS() map[string]any { return s.tokens.JWKS() }
