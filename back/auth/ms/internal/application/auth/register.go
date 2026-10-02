package auth

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"superapp/auth/internal/domain"
)

func (s *Service) Register(ctx context.Context, email, password, name string) (*domain.User, error) {
	email = domain.NormalizeEmail(email)
	parsed, parseErr := mail.ParseAddress(email)
	if parseErr != nil || parsed.Address != email || strings.TrimSpace(name) == "" || len(name) > 200 || len(email) > 320 {
		return nil, fmt.Errorf("%w: valid email and full name are required", domain.ErrInvalidArgument)
	}
	if len(password) < 12 || len(password) > 72 {
		return nil, fmt.Errorf("%w: password must be between 12 and 72 bytes", domain.ErrInvalidArgument)
	}
	hash, err := s.passwords.Hash(password)
	if err != nil {
		return nil, err
	}
	return s.users.CreateUser(ctx, s.newID(), email, hash, strings.TrimSpace(name))
}
