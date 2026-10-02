package users

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/ports/repository"
)

func (s *Service) User(ctx context.Context, id string) (*domain.User, error) {
	return s.repository.UserByID(ctx, id)
}
func (s *Service) UpdateUser(ctx context.Context, params UpdateParams) error {
	if params.Email == nil && params.FullName == nil && params.Status == nil {
		return domain.ErrInvalidArgument
	}
	if params.Email != nil {
		value := domain.NormalizeEmail(*params.Email)
		parsed, err := mail.ParseAddress(value)
		if err != nil || parsed.Address != value || len(value) > 320 {
			return fmt.Errorf("%w: valid email is required", domain.ErrInvalidArgument)
		}
		params.Email = &value
	}
	if params.FullName != nil {
		value := strings.TrimSpace(*params.FullName)
		if value == "" || len(value) > 200 {
			return fmt.Errorf("%w: valid full name is required", domain.ErrInvalidArgument)
		}
		params.FullName = &value
	}
	if params.Status != nil && *params.Status != "ACTIVE" && *params.Status != "DISABLED" && *params.Status != "LOCKED" {
		return fmt.Errorf("%w: invalid user status", domain.ErrInvalidArgument)
	}
	return s.repository.UpdateUser(ctx, repository.UpdateUserParams{
		ID: params.ID, Email: params.Email, FullName: params.FullName, Status: params.Status,
	})
}
