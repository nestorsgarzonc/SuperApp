package writer

import (
	"net/http"
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
	a.Respond(w, a.Authentication.Logout(r.Context(), in.RefreshToken), 204, nil)
}
