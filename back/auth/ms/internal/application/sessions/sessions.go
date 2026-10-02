package sessions

import (
	"context"
	"superapp/auth/internal/domain"
)

func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	return s.repository.RevokeAllSessions(ctx, userID)
}
func (s *Service) Sessions(ctx context.Context, id string) ([]domain.Session, error) {
	return s.repository.Sessions(ctx, id)
}
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string) error {
	return s.repository.RevokeSession(ctx, userID, sessionID)
}
