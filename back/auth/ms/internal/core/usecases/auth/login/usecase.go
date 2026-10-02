package login

import (
	"context"
	"time"

	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/ports/repository"
	"superapp/auth/internal/core/ports/security"
	authtokens "superapp/auth/internal/core/usecases/auth"
	"superapp/auth/internal/core/usecases/contracts"
)

type UseCase struct {
	users     repository.UserRepository
	sessions  repository.SessionRepository
	passwords security.PasswordVerifier
	tokens    security.TokenSigner
	newID     security.IDGenerator
	cfg       contracts.Config
	now       func() time.Time
}

func New(users repository.UserRepository, sessions repository.SessionRepository, passwords security.PasswordVerifier, tokens security.TokenSigner, newID security.IDGenerator, cfg contracts.Config) *UseCase {
	return &UseCase{users: users, sessions: sessions, passwords: passwords, tokens: tokens, newID: newID, cfg: cfg, now: time.Now}
}

func (u *UseCase) Execute(ctx context.Context, request Requester) (Presenter, error) {
	user, hash, err := u.users.UserByEmail(ctx, domain.NormalizeEmail(request.Email))
	if err != nil {
		return Presenter{}, err
	}
	if user == nil {
		u.passwords.Burn(request.Password)
		return Presenter{}, domain.ErrInvalidCredentials
	}
	if err = u.passwords.Verify(hash, request.Password); err != nil || user.Status != "ACTIVE" {
		return Presenter{}, domain.ErrInvalidCredentials
	}
	if err = u.users.SetLastLogin(ctx, user.ID); err != nil {
		return Presenter{}, err
	}
	sessionID := u.newID()
	refreshToken, refreshHash, err := authtokens.NewRefreshToken()
	if err != nil {
		return Presenter{}, err
	}
	now := u.now()
	accessToken, accessExpiry, err := u.tokens.Issue(user.ID, user.Email, sessionID)
	if err != nil {
		return Presenter{}, err
	}
	if err = u.sessions.CreateSession(ctx, repository.CreateSessionParams{
		SessionID: sessionID, UserID: user.ID, RefreshHash: refreshHash,
		ExpiresAt: now.Add(u.cfg.RefreshTTL), Device: request.Device,
		UserAgent: request.UserAgent, IP: request.IP,
	}); err != nil {
		return Presenter{}, err
	}
	return Presenter{User: user, Tokens: contracts.TokenPair{
		AccessToken: accessToken, RefreshToken: refreshToken, ExpiresIn: int64(accessExpiry.Sub(now).Seconds()),
	}}, nil
}
