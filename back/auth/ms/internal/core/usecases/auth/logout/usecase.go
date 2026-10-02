package logout

import (
	"context"

	"superapp/auth/internal/core/ports/repository"
	authtokens "superapp/auth/internal/core/usecases/auth"
)

type UseCase struct{ sessions repository.SessionRepository }

func New(sessions repository.SessionRepository) *UseCase { return &UseCase{sessions: sessions} }

func (u *UseCase) Execute(ctx context.Context, request Requester) (Presenter, error) {
	if err := u.sessions.RevokeRefresh(ctx, authtokens.HashRefreshToken(request.RefreshToken)); err != nil {
		return Presenter{}, err
	}
	return Presenter{}, nil
}
