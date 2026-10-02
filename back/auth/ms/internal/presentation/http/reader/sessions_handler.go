package reader

import (
	"net/http"
	shared "superapp/auth/internal/presentation/http/shared"
)

type SessionsHandler struct{ *shared.API }

func (a *SessionsHandler) listSessions(w http.ResponseWriter, r *http.Request) {
	id, ok := a.Auth(w, r)
	if !ok {
		return
	}
	v, err := a.Service.Sessions(r.Context(), id.UserID)
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, http.StatusOK, v)
}
