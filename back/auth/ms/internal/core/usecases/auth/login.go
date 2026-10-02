package auth

import (
	"context"
	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/usecases/contracts"
)

func (s *Service) Login(ctx context.Context, email, password, device, ua, ip string) (*domain.User, *contracts.TokenPair, error) {
	u, hash, err := s.users.UserByEmail(ctx, domain.NormalizeEmail(email))
	if err != nil {
		return nil, nil, err
	}
	if u == nil {
		s.passwords.Burn(password)
		return nil, nil, domain.ErrInvalidCredentials
	}
	if err = s.passwords.Verify(hash, password); err != nil {
		return nil, nil, domain.ErrInvalidCredentials
	}
	if u.Status != "ACTIVE" {
		return nil, nil, domain.ErrInvalidCredentials
	}
	if err = s.users.SetLastLogin(ctx, u.ID); err != nil {
		return nil, nil, err
	}
	pair, err := s.newSession(ctx, u, device, ua, ip)
	return u, pair, err
}
func (s *Service) newSession(ctx context.Context, u *domain.User, device, ua, ip string) (*contracts.TokenPair, error) {
	sessionID := s.newID()
	raw, hash, err := newRefresh()
	if err != nil {
		return nil, err
	}
	now := s.now()
	refreshExpiry := now.Add(s.cfg.RefreshTTL)
	access, accessExpiry, err := s.tokens.Issue(u.ID, u.Email, sessionID)
	if err != nil {
		return nil, err
	}
	if err = s.sessions.CreateSession(ctx, sessionID, u.ID, hash, refreshExpiry, device, ua, ip); err != nil {
		return nil, err
	}
	return &contracts.TokenPair{
		AccessToken: access, RefreshToken: raw, ExpiresIn: int64(accessExpiry.Sub(now).Seconds()),
	}, nil
}
