package reader

import (
	"net/http"
	"superapp/auth/internal/presentation/mapper"
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
	shared.WriteJSON(w, http.StatusOK, mapper.SessionResponses(v))
}
