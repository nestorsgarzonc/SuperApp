package roles

import (
	"context"
	"fmt"
	"strings"
	"superapp/auth/internal/domain"
)

func (s *Service) CreateRole(ctx context.Context, r domain.Role) error {
	if r.Scope != "SYSTEM" && r.Scope != "TENANT" {
		return fmt.Errorf("%w: scope must be SYSTEM or TENANT", domain.ErrInvalidArgument)
	}
	if (r.Scope == "TENANT") != (r.TenantID != nil) {
		return fmt.Errorf("%w: system roles have no tenant; tenant roles require one", domain.ErrInvalidArgument)
	}
	if !domain.ValidRoleCode(r.Code) || strings.TrimSpace(r.Name) == "" || len(r.Name) > 100 || len(r.Description) > 500 {
		return fmt.Errorf("%w: role code, name, or description is invalid", domain.ErrInvalidArgument)
	}
	r.Enabled = true
	if r.ID == "" {
		r.ID = s.newID()
	}
	return s.repository.CreateRole(ctx, r)
}
func (s *Service) Roles(ctx context.Context, tenant *string) ([]domain.Role, error) {
	return s.repository.Roles(ctx, tenant)
}
func (s *Service) RoleByID(ctx context.Context, id string, tenant *string) (*domain.Role, error) {
	return s.repository.RoleByID(ctx, id, tenant)
}
func (s *Service) UpdateRole(ctx context.Context, r domain.Role, tenant *string) error {
	if strings.TrimSpace(r.Name) == "" || len(r.Name) > 100 || len(r.Description) > 500 {
		return fmt.Errorf("%w: role name or description is invalid", domain.ErrInvalidArgument)
	}
	return s.repository.UpdateRole(ctx, r, tenant)
}
func (s *Service) DeleteRole(ctx context.Context, id string, tenant *string) error {
	return s.repository.DeleteRole(ctx, id, tenant)
}
