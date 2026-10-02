package permissions

import (
	"context"
	"superapp/auth/internal/core/domain"
)

func (s *Service) CreatePermissionFromRequest(ctx context.Context, request Requester) (Presenter, error) {
	created, err := s.CreatePermission(ctx, domain.Permission{Code: request.Code, Description: request.Description})
	if err != nil {
		return Presenter{}, err
	}
	return Presenter{ID: created.ID, Code: created.Code, Description: created.Description}, nil
}
