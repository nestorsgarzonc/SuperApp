package writer

import (
	"net/http"
	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/usecases/auth/refresh"
	dtoAuth "superapp/auth/internal/presentation/dto/auth"
	shared "superapp/auth/internal/presentation/shared"
)

func (a *Handlers) refresh(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !shared.Decode(w, r, &in) {
		return
	}
	if len(in.RefreshToken) > 512 {
		a.Fail(w, domain.ErrInvalidToken)
		return
	}
	result, err := a.Refresh.Execute(r.Context(), refresh.Requester{RefreshToken: in.RefreshToken})
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 200, dtoAuth.FromDomain(&result.Tokens))
}
