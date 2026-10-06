package repository

import (
	"context"
	"superapp/auth/internal/core/domain"
)

type PermissionRepository interface {
	Permissions(context.Context) ([]domain.Permission, error)
	CreatePermission(context.Context, domain.Permission) error
	PermissionByID(context.Context, string) (*domain.Permission, error)
	UpdatePermission(context.Context, domain.Permission) error
	DeletePermission(context.Context, string) error
}
