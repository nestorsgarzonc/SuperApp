package repository

import (
	"context"
	"superapp/auth/internal/core/domain"
)

type AuthorizationRepository interface {
	GrantPermission(context.Context, RolePermissionParams) error
	RevokePermission(context.Context, RolePermissionParams) error
	AssignRole(context.Context, AssignRoleParams) error
	RevokeRole(context.Context, RevokeRoleParams) error
	EffectiveRoles(context.Context, string, *string) ([]domain.Role, error)
	EffectivePermissions(context.Context, string, *string) ([]string, error)
	PermissionAllowed(context.Context, PermissionAllowedParams) (bool, error)
}

type RolePermissionParams struct {
	RoleID       string
	PermissionID string
	TenantID     *string
}

type AssignRoleParams struct {
	UserID   string
	RoleID   string
	TenantID *string
	ActorID  string
}

type RevokeRoleParams struct {
	UserID   string
	RoleID   string
	TenantID *string
}

type PermissionAllowedParams struct {
	UserID         string
	PermissionCode string
	TenantID       *string
}
