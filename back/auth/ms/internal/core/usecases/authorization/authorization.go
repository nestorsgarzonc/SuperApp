package authorization

import (
	"context"
	"superapp/auth/internal/core/domain"
)

func (s *Service) HasPermission(ctx context.Context, userID, permission string, tenant *string) (bool, error) {
	return s.repository.PermissionAllowed(ctx, userID, permission, tenant)
}
func (s *Service) EffectivePermissions(ctx context.Context, userID string, tenant *string) ([]string, error) {
	return s.repository.EffectivePermissions(ctx, userID, tenant)
}
func (s *Service) EffectiveRoles(ctx context.Context, userID string, tenant *string) ([]domain.Role, error) {
	return s.repository.EffectiveRoles(ctx, userID, tenant)
}
func (s *Service) GrantPermission(ctx context.Context, roleID, permissionID string, tenant *string) error {
	return s.repository.GrantPermission(ctx, roleID, permissionID, tenant)
}
func (s *Service) RevokePermission(ctx context.Context, roleID, permissionID string, tenant *string) error {
	return s.repository.RevokePermission(ctx, roleID, permissionID, tenant)
}
func (s *Service) AssignRole(ctx context.Context, userID, roleID string, tenant *string, actor string) error {
	return s.repository.AssignRole(ctx, userID, roleID, tenant, actor)
}
func (s *Service) RevokeRole(ctx context.Context, userID, roleID string, tenant *string) error {
	return s.repository.RevokeRole(ctx, userID, roleID, tenant)
}
