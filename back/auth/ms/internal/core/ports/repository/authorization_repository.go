package repository

import (
	"context"
	"superapp/auth/internal/core/domain"
)

type AuthorizationRepository interface {
	GrantPermission(context.Context, string, string, *string) error
	RevokePermission(context.Context, string, string, *string) error
	AssignRole(context.Context, string, string, *string, string) error
	RevokeRole(context.Context, string, string, *string) error
	EffectiveRoles(context.Context, string, *string) ([]domain.Role, error)
	EffectivePermissions(context.Context, string, *string) ([]string, error)
	PermissionAllowed(context.Context, string, string, *string) (bool, error)
}
