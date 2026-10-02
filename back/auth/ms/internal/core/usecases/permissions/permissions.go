package permissions

import (
	"context"
	"fmt"
	"superapp/auth/internal/core/domain"
)

func (s *Service) CreatePermission(ctx context.Context, request CreateRequester) (Presenter, error) {
	p := domain.Permission{Code: request.Code, Description: request.Description}
	if !domain.ValidPermissionCode(p.Code) || len(p.Description) > 500 {
		return Presenter{}, fmt.Errorf("%w: permission code or description is invalid", domain.ErrInvalidArgument)
	}
	if p.ID == "" {
		p.ID = s.newID()
	}
	if err := s.repository.CreatePermission(ctx, p); err != nil {
		return Presenter{}, err
	}
	return present(p), nil
}

func (s *Service) ListPermissions(ctx context.Context, _ ListRequester) (ListPresenter, error) {
	permissions, err := s.repository.Permissions(ctx)
	if err != nil {
		return ListPresenter{}, err
	}
	result := ListPresenter{Permissions: make([]Presenter, 0, len(permissions))}
	for _, permission := range permissions {
		result.Permissions = append(result.Permissions, present(permission))
	}
	return result, nil
}

func (s *Service) GetPermission(ctx context.Context, request GetRequester) (Presenter, error) {
	permission, err := s.repository.PermissionByID(ctx, request.PermissionID)
	if err != nil {
		return Presenter{}, err
	}
	return present(*permission), nil
}

func (s *Service) UpdatePermission(ctx context.Context, request UpdateRequester) (Presenter, error) {
	if request.Code == nil && request.Description == nil {
		return Presenter{}, domain.ErrInvalidArgument
	}
	permission, err := s.repository.PermissionByID(ctx, request.PermissionID)
	if err != nil {
		return Presenter{}, err
	}
	if request.Code != nil {
		permission.Code = *request.Code
	}
	if request.Description != nil {
		permission.Description = *request.Description
	}
	if !domain.ValidPermissionCode(permission.Code) || len(permission.Description) > 500 {
		return Presenter{}, fmt.Errorf("%w: permission code or description is invalid", domain.ErrInvalidArgument)
	}
	if err := s.repository.UpdatePermission(ctx, *permission); err != nil {
		return Presenter{}, err
	}
	return present(*permission), nil
}

func (s *Service) DeletePermission(ctx context.Context, request DeleteRequester) (DeletePresenter, error) {
	return DeletePresenter{}, s.repository.DeletePermission(ctx, request.PermissionID)
}

func present(permission domain.Permission) Presenter {
	return Presenter{ID: permission.ID, Code: permission.Code, Description: permission.Description}
}
