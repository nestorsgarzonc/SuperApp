package reader

import (
	"net/http"
	shared "superapp/auth/internal/presentation/http/shared"
)

func (a *Handlers) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.MethodNotAllowed(w)
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
