package users

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/ports/repository"
)

func (s *Service) GetUser(ctx context.Context, request GetRequester) (Presenter, error) {
	user, err := s.repository.UserByID(ctx, request.UserID)
	if err != nil {
		return Presenter{}, err
	}
	return Presenter{ID: user.ID, Email: user.Email, FullName: user.FullName, Status: user.Status, CreatedAt: user.CreatedAt}, nil
}
func (s *Service) UpdateUser(ctx context.Context, params UpdateRequester) (UpdatePresenter, error) {
	if params.Email == nil && params.FullName == nil && params.Status == nil {
		return UpdatePresenter{}, domain.ErrInvalidArgument
	}
	if params.Email != nil {
		value := domain.NormalizeEmail(*params.Email)
		parsed, err := mail.ParseAddress(value)
		if err != nil || parsed.Address != value || len(value) > 320 {
			return UpdatePresenter{}, fmt.Errorf("%w: valid email is required", domain.ErrInvalidArgument)
		}
		params.Email = &value
	}
	if params.FullName != nil {
		value := strings.TrimSpace(*params.FullName)
		if value == "" || len(value) > 200 {
			return UpdatePresenter{}, fmt.Errorf("%w: valid full name is required", domain.ErrInvalidArgument)
		}
		params.FullName = &value
	}
	if params.Status != nil && *params.Status != "ACTIVE" && *params.Status != "DISABLED" && *params.Status != "LOCKED" {
		return UpdatePresenter{}, fmt.Errorf("%w: invalid user status", domain.ErrInvalidArgument)
	}
	if err := s.repository.UpdateUser(ctx, repository.UpdateUserParams{
		ID: params.ID, Email: params.Email, FullName: params.FullName, Status: params.Status,
	}); err != nil {
		return UpdatePresenter{}, err
	}
	return UpdatePresenter{}, nil
}
