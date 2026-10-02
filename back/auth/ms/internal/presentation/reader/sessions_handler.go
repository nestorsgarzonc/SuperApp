package reader

import (
	"net/http"
	appsessions "superapp/auth/internal/core/usecases/sessions"
	"superapp/auth/internal/presentation/mapper"
	shared "superapp/auth/internal/presentation/shared"
)

type SessionsHandler struct{ *shared.API }

func (a *SessionsHandler) listSessions(w http.ResponseWriter, r *http.Request) {
	id, ok := a.Auth(w, r)
	if !ok {
		return
	}
	v, err := a.Sessions.ListSessions(r.Context(), appsessions.ListRequester{UserID: id.UserID})
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, http.StatusOK, mapper.SessionResponses(v))
}
