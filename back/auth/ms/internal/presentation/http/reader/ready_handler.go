package reader

import (
	"net/http"
	shared "superapp/auth/internal/presentation/http/shared"
)

func (a *Handlers) ready(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		shared.MethodNotAllowed(w)
		return
	}
	if err := a.Service.Ready(r.Context()); err != nil {
		shared.WriteError(w, 503, "not_ready", "Auth dependencies unavailable")
		return
	}
	shared.WriteJSON(w, 200, map[string]string{"status": "ready"})
}
