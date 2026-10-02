package writer

import (
	"net/http"
	"superapp/auth/internal/core/usecases/auth/logout"
	shared "superapp/auth/internal/presentation/shared"
)

func (a *Handlers) logout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !shared.Decode(w, r, &in) {
		return
	}
	if len(in.RefreshToken) > 512 {
		w.WriteHeader(204)
		return
	}
	_, err := a.Logout.Execute(r.Context(), logout.Requester{RefreshToken: in.RefreshToken})
	a.Respond(w, err, 204, nil)
}
