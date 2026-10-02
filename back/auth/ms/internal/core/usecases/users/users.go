package users

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"superapp/auth/internal/core/domain"
)

func (s *Service) User(ctx context.Context, id string) (*domain.User, error) {
	return s.repository.UserByID(ctx, id)
}
func (s *Service) UpdateUser(ctx context.Context, id string, email, name, status *string) error {
	if email == nil && name == nil && status == nil {
		return domain.ErrInvalidArgument
	}
	if email != nil {
		value := domain.NormalizeEmail(*email)
		parsed, err := mail.ParseAddress(value)
		if err != nil || parsed.Address != value || len(value) > 320 {
			return fmt.Errorf("%w: valid email is required", domain.ErrInvalidArgument)
		}
		email = &value
	}
	if name != nil {
		value := strings.TrimSpace(*name)
		if value == "" || len(value) > 200 {
			return fmt.Errorf("%w: valid full name is required", domain.ErrInvalidArgument)
		}
		name = &value
	}
	if status != nil && *status != "ACTIVE" && *status != "DISABLED" && *status != "LOCKED" {
		return fmt.Errorf("%w: invalid user status", domain.ErrInvalidArgument)
	}
	return s.repository.UpdateUser(ctx, id, email, name, status)
}
