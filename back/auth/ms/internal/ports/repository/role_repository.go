package repository

import (
	"context"
	"superapp/auth/internal/domain"
)

type RoleRepository interface {
	Roles(context.Context, *string) ([]domain.Role, error)
	CreateRole(context.Context, domain.Role) error
	RoleByID(context.Context, string, *string) (*domain.Role, error)
	UpdateRole(context.Context, domain.Role, *string) error
	DeleteRole(context.Context, string, *string) error
}
