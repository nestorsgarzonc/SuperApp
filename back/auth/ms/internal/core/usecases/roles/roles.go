package roles

import (
	"context"
	"fmt"
	"strings"
	"superapp/auth/internal/core/domain"
)

func (s *Service) CreateRole(ctx context.Context, request CreateRequester) (Presenter, error) {
	r := domain.Role{Code: request.Code, Name: request.Name, Description: request.Description, Scope: request.Scope, TenantID: request.TenantID}
	if r.Scope != "SYSTEM" && r.Scope != "TENANT" {
		return Presenter{}, fmt.Errorf("%w: scope must be SYSTEM or TENANT", domain.ErrInvalidArgument)
	}
	if (r.Scope == "TENANT") != (r.TenantID != nil) {
		return Presenter{}, fmt.Errorf("%w: system roles have no tenant; tenant roles require one", domain.ErrInvalidArgument)
	}
	if !domain.ValidRoleCode(r.Code) || strings.TrimSpace(r.Name) == "" || len(r.Name) > 100 || len(r.Description) > 500 {
		return Presenter{}, fmt.Errorf("%w: role code, name, or description is invalid", domain.ErrInvalidArgument)
	}
	r.Enabled = true
	if r.ID == "" {
		r.ID = s.newID()
	}
	if err := s.repository.CreateRole(ctx, r); err != nil {
		return Presenter{}, err
	}
	return present(r), nil
}

func (s *Service) ListRoles(ctx context.Context, request ListRequester) (ListPresenter, error) {
	roles, err := s.repository.Roles(ctx, request.TenantID)
	if err != nil {
		return ListPresenter{}, err
	}
	result := ListPresenter{Roles: make([]Presenter, 0, len(roles))}
	for _, role := range roles {
		result.Roles = append(result.Roles, present(role))
	}
	return result, nil
}

func (s *Service) GetRole(ctx context.Context, request GetRequester) (Presenter, error) {
	role, err := s.repository.RoleByID(ctx, request.RoleID, request.TenantID)
	if err != nil {
		return Presenter{}, err
	}
	return present(*role), nil
}

func (s *Service) UpdateRole(ctx context.Context, request UpdateRequester) (Presenter, error) {
	if request.Name == nil && request.Description == nil && request.Enabled == nil {
		return Presenter{}, domain.ErrInvalidArgument
	}
	role, err := s.repository.RoleByID(ctx, request.RoleID, request.TenantID)
	if err != nil {
		return Presenter{}, err
	}
	if request.Name != nil {
		role.Name = *request.Name
	}
	if request.Description != nil {
		role.Description = *request.Description
	}
	if request.Enabled != nil {
		role.Enabled = *request.Enabled
	}
	if strings.TrimSpace(role.Name) == "" || len(role.Name) > 100 || len(role.Description) > 500 {
		return Presenter{}, fmt.Errorf("%w: role name or description is invalid", domain.ErrInvalidArgument)
	}
	if err := s.repository.UpdateRole(ctx, *role, request.TenantID); err != nil {
		return Presenter{}, err
	}
	return present(*role), nil
}

func (s *Service) DeleteRole(ctx context.Context, request DeleteRequester) (DeletePresenter, error) {
	return DeletePresenter{}, s.repository.DeleteRole(ctx, request.RoleID, request.TenantID)
}

func present(role domain.Role) Presenter {
	return Presenter{
		ID: role.ID, Code: role.Code, Name: role.Name, Description: role.Description,
		Scope: role.Scope, TenantID: role.TenantID, Enabled: role.Enabled,
	}
}
