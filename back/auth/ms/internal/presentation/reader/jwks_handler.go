package reader

import (
	"net/http"
	shared "superapp/auth/internal/presentation/shared"
)

func (a *Handlers) jwks(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		shared.MethodNotAllowed(w)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	shared.WriteJSON(w, 200, a.JWKS.JWKS())
}
