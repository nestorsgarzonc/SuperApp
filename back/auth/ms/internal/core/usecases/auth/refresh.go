package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"superapp/auth/internal/core/usecases/contracts"
)

func (s *Service) Refresh(ctx context.Context, raw string) (*contracts.TokenPair, error) {
	oldHash := hashRefresh(raw)
	newRaw, newHash, err := newRefresh()
	if err != nil {
		return nil, err
	}
	now := s.now()
	sessionID, userID, email, err := s.sessions.RotateRefresh(ctx, oldHash, newHash, s.newID(), now.Add(s.cfg.RefreshTTL), now)
	if err != nil {
		return nil, err
	}
	access, expiry, err := s.tokens.Issue(userID, email, sessionID)
	if err != nil {
		return nil, err
	}
	return &contracts.TokenPair{
		AccessToken: access, RefreshToken: newRaw, ExpiresIn: int64(expiry.Sub(now).Seconds()),
	}, nil
}
func newRefresh() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	hash = hashRefresh(raw)
	return
}
func hashRefresh(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
