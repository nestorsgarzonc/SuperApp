package writer

import (
	"net/http"
	"superapp/auth/internal/domain"
	shared "superapp/auth/internal/presentation/http/shared"
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
	p, err := a.Service.Refresh(r.Context(), in.RefreshToken)
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 200, p)
}
