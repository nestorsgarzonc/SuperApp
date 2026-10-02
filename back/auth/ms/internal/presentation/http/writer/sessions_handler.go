package writer

import (
	"net/http"
	"strings"
	"superapp/auth/internal/domain"
	shared "superapp/auth/internal/presentation/http/shared"
)

type SessionsHandler struct{ *shared.API }

func (a *SessionsHandler) logoutAll(w http.ResponseWriter, r *http.Request) {
	id, ok := a.Auth(w, r)
	if !ok {
		return
	}
	a.Respond(w, a.Service.LogoutAll(r.Context(), id.UserID), http.StatusNoContent, nil)
}
func (a *SessionsHandler) revokeSession(w http.ResponseWriter, r *http.Request) {
	id, ok := a.Auth(w, r)
	if !ok {
		return
	}
	id2 := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")
	if !shared.ValidUUID(id2) {
		a.Fail(w, domain.ErrInvalidArgument)
		return
	}
	a.Respond(w, a.Service.RevokeSession(r.Context(), id.UserID, id2), http.StatusNoContent, nil)
}
