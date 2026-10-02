package auth

import (
	"time"

	"superapp/auth/internal/application/contracts"
	"superapp/auth/internal/ports/repository"
)

type Service struct {
	users     repository.UserRepository
	sessions  repository.SessionRepository
	passwords contracts.PasswordHasher
	tokens    contracts.TokenIssuer
	ids       contracts.IDGenerator
	cfg       contracts.Config
	now       func() time.Time
}

func New(users repository.UserRepository, sessions repository.SessionRepository, passwords contracts.PasswordHasher, tokens contracts.TokenIssuer, ids contracts.IDGenerator, cfg contracts.Config) *Service {
	return &Service{users: users, sessions: sessions, passwords: passwords, tokens: tokens, ids: ids, cfg: cfg, now: time.Now}
}
func (s *Service) Authenticate(raw string) (*contracts.TokenClaims, error) {
	return s.tokens.Verify(raw)
}
func (s *Service) JWKS() map[string]any { return s.tokens.JWKS() }
