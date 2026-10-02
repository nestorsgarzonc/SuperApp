package repository

import (
	"context"
	"superapp/auth/internal/core/domain"
)

type CreateUserParams struct {
	ID           string
	Email        string
	PasswordHash string
	FullName     string
}

type UpdateUserParams struct {
	ID       string
	Email    *string
	FullName *string
	Status   *string
}

type UserRepository interface {
	CreateUser(context.Context, CreateUserParams) (*domain.User, error)
	UserByEmail(context.Context, string) (*domain.User, string, error)
	UserByID(context.Context, string) (*domain.User, error)
	UpdateUser(context.Context, UpdateUserParams) error
	SetLastLogin(context.Context, string) error
}
