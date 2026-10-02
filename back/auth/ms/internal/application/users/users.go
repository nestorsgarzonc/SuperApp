package users

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"superapp/auth/internal/domain"
)

func (s *Service) User(ctx context.Context, id string) (*domain.User, error) {
	return s.repository.UserByID(ctx, id)
}
func (s *Service) UpdateUser(ctx context.Context, id, email, name string) error {
	email = domain.NormalizeEmail(email)
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || strings.TrimSpace(name) == "" || len(name) > 200 || len(email) > 320 {
		return fmt.Errorf("%w: valid email and full name are required", domain.ErrInvalidArgument)
	}
	return s.repository.UpdateUser(ctx, id, email, strings.TrimSpace(name))
}
func (s *Service) SetUserStatus(ctx context.Context, userID, status string) error {
	if status != "ACTIVE" && status != "DISABLED" && status != "LOCKED" {
		return fmt.Errorf("%w: invalid user status", domain.ErrInvalidArgument)
	}
	return s.repository.SetUserStatus(ctx, userID, status)
}
