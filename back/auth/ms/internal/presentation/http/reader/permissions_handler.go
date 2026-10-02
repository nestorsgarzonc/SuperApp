package reader

import (
	"net/http"
	shared "superapp/auth/internal/presentation/http/shared"
)

func (a *Handlers) listPermissions(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.RequirePermission(w, r, "auth.roles.read"); !ok {
		return
	}
	v, err := a.Service.Permissions(r.Context())
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 200, v)
}
