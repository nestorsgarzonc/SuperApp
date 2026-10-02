package reader

import (
	"net/http"
	dtoSessions "superapp/auth/internal/presentation/dto/sessions"
	shared "superapp/auth/internal/presentation/shared"
)

type SessionsHandler struct{ *shared.API }

func (a *SessionsHandler) listSessions(w http.ResponseWriter, r *http.Request) {
	id, ok := a.Auth(w, r)
	if !ok {
		return
	}
	v, err := a.Sessions.Sessions(r.Context(), id.UserID)
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, http.StatusOK, dtoSessions.ListFromDomain(v))
}
