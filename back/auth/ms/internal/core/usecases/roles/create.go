package roles

import (
	"context"
	"superapp/auth/internal/core/domain"
)

func (s *Service) CreateRoleFromRequest(ctx context.Context, request Requester) (Presenter, error) {
	created, err := s.CreateRole(ctx, domain.Role{
		Code: request.Code, Name: request.Name, Description: request.Description,
		Scope: request.Scope, TenantID: request.TenantID,
	})
	if err != nil {
		return Presenter{}, err
	}
	return Presenter{
		ID: created.ID, Code: created.Code, Name: created.Name,
		Description: created.Description, Scope: created.Scope,
		TenantID: created.TenantID, Enabled: created.Enabled,
	}, nil
}
