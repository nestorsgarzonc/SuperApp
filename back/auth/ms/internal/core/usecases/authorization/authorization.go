package authorization

import (
	"context"
	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/ports/repository"
)

func (s *Service) CheckPermission(ctx context.Context, params CheckPermissionRequester) (CheckPermissionPresenter, error) {
	if !domain.ValidPermissionCode(params.PermissionCode) {
		return CheckPermissionPresenter{}, domain.ErrInvalidArgument
	}
	allowed, err := s.repository.PermissionAllowed(ctx, repository.PermissionAllowedParams{
		UserID: params.UserID, PermissionCode: params.PermissionCode, TenantID: params.TenantID,
	})
	return CheckPermissionPresenter{Allowed: allowed}, err
}
func (s *Service) EffectivePermissions(ctx context.Context, params EffectivePermissionsRequester) (EffectivePermissionsPresenter, error) {
	permissions, err := s.repository.EffectivePermissions(ctx, params.UserID, params.TenantID)
	if err != nil {
		return EffectivePermissionsPresenter{}, err
	}
	return EffectivePermissionsPresenter{Permissions: permissions}, nil
}
func (s *Service) EffectiveRoles(ctx context.Context, params EffectiveRolesRequester) (EffectiveRolesPresenter, error) {
	roles, err := s.repository.EffectiveRoles(ctx, params.UserID, params.TenantID)
	if err != nil {
		return EffectiveRolesPresenter{}, err
	}
	result := EffectiveRolesPresenter{Roles: make([]RolePresenter, 0, len(roles))}
	for _, role := range roles {
		result.Roles = append(result.Roles, RolePresenter{
			ID: role.ID, Code: role.Code, Name: role.Name, Description: role.Description,
			Scope: role.Scope, TenantID: role.TenantID, Enabled: role.Enabled,
		})
	}
	return result, nil
}
func (s *Service) GrantPermission(ctx context.Context, params RolePermissionParams) (Presenter, error) {
	err := s.repository.GrantPermission(ctx, repository.RolePermissionParams{
		RoleID: params.RoleID, PermissionID: params.PermissionID, TenantID: params.TenantID,
	})
	return Presenter{}, err
}
func (s *Service) RevokePermission(ctx context.Context, params RolePermissionParams) (Presenter, error) {
	err := s.repository.RevokePermission(ctx, repository.RolePermissionParams{
		RoleID: params.RoleID, PermissionID: params.PermissionID, TenantID: params.TenantID,
	})
	return Presenter{}, err
}
func (s *Service) AssignRole(ctx context.Context, params AssignRoleParams) (Presenter, error) {
	err := s.repository.AssignRole(ctx, repository.AssignRoleParams{
		UserID: params.UserID, RoleID: params.RoleID, TenantID: params.TenantID, ActorID: params.ActorID,
	})
	return Presenter{}, err
}
func (s *Service) RevokeRole(ctx context.Context, params RevokeRoleParams) (Presenter, error) {
	err := s.repository.RevokeRole(ctx, repository.RevokeRoleParams{
		UserID: params.UserID, RoleID: params.RoleID, TenantID: params.TenantID,
	})
	return Presenter{}, err
}
