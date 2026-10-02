package permissions

import (
	"context"
	"fmt"
	"superapp/auth/internal/core/domain"
)

func (s *Service) CreatePermission(ctx context.Context, p domain.Permission) error {
	if !domain.ValidPermissionCode(p.Code) || len(p.Description) > 500 {
		return fmt.Errorf("%w: permission code or description is invalid", domain.ErrInvalidArgument)
	}
	if p.ID == "" {
		p.ID = s.newID()
	}
	return s.repository.CreatePermission(ctx, p)
}
func (s *Service) Permissions(ctx context.Context) ([]domain.Permission, error) {
	return s.repository.Permissions(ctx)
}
func (s *Service) PermissionByID(ctx context.Context, id string) (*domain.Permission, error) {
	return s.repository.PermissionByID(ctx, id)
}
func (s *Service) UpdatePermission(ctx context.Context, p domain.Permission) error {
	if !domain.ValidPermissionCode(p.Code) || len(p.Description) > 500 {
		return fmt.Errorf("%w: permission code or description is invalid", domain.ErrInvalidArgument)
	}
	return s.repository.UpdatePermission(ctx, p)
}
func (s *Service) DeletePermission(ctx context.Context, id string) error {
	return s.repository.DeletePermission(ctx, id)
}
