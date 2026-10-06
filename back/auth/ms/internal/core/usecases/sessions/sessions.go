package sessions

import (
	"context"
	"superapp/auth/internal/core/ports/repository"
)

func (s *Service) LogoutAll(ctx context.Context, request LogoutAllRequester) (LogoutAllPresenter, error) {
	return LogoutAllPresenter{}, s.repository.RevokeAllSessions(ctx, request.UserID)
}
func (s *Service) ListSessions(ctx context.Context, request ListRequester) (ListPresenter, error) {
	values, err := s.repository.Sessions(ctx, request.UserID)
	if err != nil {
		return ListPresenter{}, err
	}
	result := ListPresenter{Sessions: make([]Presenter, 0, len(values))}
	for _, value := range values {
		result.Sessions = append(result.Sessions, Presenter{
			ID: value.ID, Device: value.Device, CreatedAt: value.CreatedAt, ExpiresAt: value.ExpiresAt,
		})
	}
	return result, nil
}
func (s *Service) RevokeSession(ctx context.Context, params RevokeParams) (RevokePresenter, error) {
	err := s.repository.RevokeSession(ctx, repository.RevokeSessionParams{
		UserID: params.UserID, SessionID: params.SessionID,
	})
	return RevokePresenter{}, err
}
