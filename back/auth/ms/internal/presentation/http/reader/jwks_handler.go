package reader

import (
	"net/http"
	shared "superapp/auth/internal/presentation/http/shared"
)

func (a *Handlers) jwks(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		shared.MethodNotAllowed(w)
		return
	}
	if jwkProvider, ok := a.Service.(interface{ JWKS() map[string]any }); ok {
		w.Header().Set("Cache-Control", "public, max-age=300")
		shared.WriteJSON(w, 200, jwkProvider.JWKS())
		return
	}
	shared.WriteError(w, 500, "jwks_unavailable", "Signing keys unavailable")
}
