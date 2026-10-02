package register

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/ports/repository"
	"superapp/auth/internal/core/ports/security"
)

type UseCase struct {
	users     repository.UserRepository
	passwords security.PasswordHasher
	newID     security.IDGenerator
}

func New(users repository.UserRepository, passwords security.PasswordHasher, newID security.IDGenerator) *UseCase {
	return &UseCase{users: users, passwords: passwords, newID: newID}
}

func (u *UseCase) Execute(ctx context.Context, request Requester) (Presenter, error) {
	email := domain.NormalizeEmail(request.Email)
	parsed, parseErr := mail.ParseAddress(email)
	if parseErr != nil || parsed.Address != email || strings.TrimSpace(request.Name) == "" || len(request.Name) > 200 || len(email) > 320 {
		return Presenter{}, fmt.Errorf("%w: valid email and full name are required", domain.ErrInvalidArgument)
	}
	if len(request.Password) < 12 || len(request.Password) > 72 {
		return Presenter{}, fmt.Errorf("%w: password must be between 12 and 72 bytes", domain.ErrInvalidArgument)
	}
	hash, err := u.passwords.Hash(request.Password)
	if err != nil {
		return Presenter{}, err
	}
	user, err := u.users.CreateUser(ctx, u.newID(), email, hash, strings.TrimSpace(request.Name))
	if err != nil {
		return Presenter{}, err
	}
	return Presenter{User: user}, nil
}
