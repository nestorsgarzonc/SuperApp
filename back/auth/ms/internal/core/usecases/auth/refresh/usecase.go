package refresh

import (
	"context"
	"time"

	"superapp/auth/internal/core/ports/repository"
	"superapp/auth/internal/core/ports/security"
	authtokens "superapp/auth/internal/core/usecases/auth"
	"superapp/auth/internal/core/usecases/contracts"
)

type UseCase struct {
	sessions repository.SessionRepository
	tokens   security.TokenSigner
	newID    security.IDGenerator
	cfg      contracts.Config
	now      func() time.Time
}

func New(sessions repository.SessionRepository, tokens security.TokenSigner, newID security.IDGenerator, cfg contracts.Config) *UseCase {
	return &UseCase{sessions: sessions, tokens: tokens, newID: newID, cfg: cfg, now: time.Now}
}

func (u *UseCase) Execute(ctx context.Context, request Requester) (Presenter, error) {
	oldHash := authtokens.HashRefreshToken(request.RefreshToken)
	newRaw, newHash, err := authtokens.NewRefreshToken()
	if err != nil {
		return Presenter{}, err
	}
	now := u.now()
	rotated, err := u.sessions.RotateRefresh(ctx, repository.RotateRefreshParams{
		OldHash: oldHash, NewHash: newHash, NewTokenID: u.newID(),
		ExpiresAt: now.Add(u.cfg.RefreshTTL), Now: now,
	})
	if err != nil {
		return Presenter{}, err
	}
	access, expiry, err := u.tokens.Issue(rotated.UserID, rotated.SessionID)
	if err != nil {
		return Presenter{}, err
	}
	return Presenter{Tokens: contracts.TokenPair{
		AccessToken: access, RefreshToken: newRaw, ExpiresIn: int64(expiry.Sub(now).Seconds()),
	}}, nil
}
