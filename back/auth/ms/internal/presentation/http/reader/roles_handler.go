package reader

import (
	"net/http"
	shared "superapp/auth/internal/presentation/http/shared"
)

func (a *Handlers) listRoles(w http.ResponseWriter, r *http.Request) {
	id, ok := a.RequirePermission(w, r, "auth.roles.read")
	if !ok {
		return
	}
	tenant, err := a.Tenant(r, id.UserID)
	if err != nil {
		a.Fail(w, err)
		return
	}
	v, err := a.Service.Roles(r.Context(), tenant)
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 200, v)
}
