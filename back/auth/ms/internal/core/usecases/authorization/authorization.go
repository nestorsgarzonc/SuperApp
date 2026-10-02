package authorization

import (
	"context"
	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/ports/repository"
)

func (s *Service) HasPermission(ctx context.Context, params HasPermissionParams) (bool, error) {
	return s.repository.PermissionAllowed(ctx, repository.PermissionAllowedParams{
		UserID: params.UserID, PermissionCode: params.PermissionCode, TenantID: params.TenantID,
	})
}
func (s *Service) EffectivePermissions(ctx context.Context, userID string, tenant *string) ([]string, error) {
	return s.repository.EffectivePermissions(ctx, userID, tenant)
}
func (s *Service) EffectiveRoles(ctx context.Context, userID string, tenant *string) ([]domain.Role, error) {
	return s.repository.EffectiveRoles(ctx, userID, tenant)
}
func (s *Service) GrantPermission(ctx context.Context, params RolePermissionParams) error {
	return s.repository.GrantPermission(ctx, repository.RolePermissionParams{
		RoleID: params.RoleID, PermissionID: params.PermissionID, TenantID: params.TenantID,
	})
}
func (s *Service) RevokePermission(ctx context.Context, params RolePermissionParams) error {
	return s.repository.RevokePermission(ctx, repository.RolePermissionParams{
		RoleID: params.RoleID, PermissionID: params.PermissionID, TenantID: params.TenantID,
	})
}
func (s *Service) AssignRole(ctx context.Context, params AssignRoleParams) error {
	return s.repository.AssignRole(ctx, repository.AssignRoleParams{
		UserID: params.UserID, RoleID: params.RoleID, TenantID: params.TenantID, ActorID: params.ActorID,
	})
}
func (s *Service) RevokeRole(ctx context.Context, params RevokeRoleParams) error {
	return s.repository.RevokeRole(ctx, repository.RevokeRoleParams{
		UserID: params.UserID, RoleID: params.RoleID, TenantID: params.TenantID,
	})
}
