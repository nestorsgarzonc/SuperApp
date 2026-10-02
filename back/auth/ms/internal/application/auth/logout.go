package auth

import (
	"context"
)

func (s *Service) Logout(ctx context.Context, raw string) error {
	return s.sessions.RevokeRefresh(ctx, hashRefresh(raw))
}
