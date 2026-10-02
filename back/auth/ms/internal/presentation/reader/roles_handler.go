package reader

import (
	"net/http"
	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/presentation/mapper"
	shared "superapp/auth/internal/presentation/shared"
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
	v, err := a.Roles.Roles(r.Context(), tenant)
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 200, mapper.RoleResponses(v))
}

func (a *Handlers) roleRoute(w http.ResponseWriter, r *http.Request, p []string) {
	if len(p) != 2 {
		shared.WriteError(w, http.StatusNotFound, "not_found", "Route not found")
		return
	}
	id, ok := a.RequirePermission(w, r, "auth.roles.read")
	if !ok {
		return
	}
	tenant, err := a.Tenant(r, id.UserID)
	if err != nil {
		a.Fail(w, err)
		return
	}
	if !shared.ValidUUID(p[1]) {
		a.Fail(w, domain.ErrInvalidArgument)
		return
	}
	role, err := a.Roles.RoleByID(r.Context(), p[1], tenant)
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, http.StatusOK, mapper.RoleResponse(role))
}
