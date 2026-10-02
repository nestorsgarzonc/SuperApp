package repository

import (
	"context"
	"superapp/auth/internal/domain"
)

type UserRepository interface {
	CreateUser(context.Context, string, string, string, string) (*domain.User, error)
	UserByEmail(context.Context, string) (*domain.User, string, error)
	UserByID(context.Context, string) (*domain.User, error)
	UpdateUser(context.Context, string, string, string) error
	SetLastLogin(context.Context, string) error
	SetUserStatus(context.Context, string, string) error
}
