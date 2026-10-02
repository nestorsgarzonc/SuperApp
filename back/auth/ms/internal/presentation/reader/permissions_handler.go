package reader

import (
	"net/http"
	"superapp/auth/internal/core/domain"
	apppermissions "superapp/auth/internal/core/usecases/permissions"
	"superapp/auth/internal/presentation/mapper"
	shared "superapp/auth/internal/presentation/shared"
)

func (a *Handlers) listPermissions(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.RequirePermission(w, r, "auth.roles.read"); !ok {
		return
	}
	v, err := a.Permissions.ListPermissions(r.Context(), apppermissions.ListRequester{})
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 200, mapper.PermissionResponses(v))
}

func (a *Handlers) permissionRoute(w http.ResponseWriter, r *http.Request, p []string) {
	if len(p) != 2 {
		shared.WriteError(w, http.StatusNotFound, "not_found", "Route not found")
		return
	}
	if _, ok := a.RequirePermission(w, r, "auth.roles.read"); !ok {
		return
	}
	if !shared.ValidUUID(p[1]) {
		a.Fail(w, domain.ErrInvalidArgument)
		return
	}
	permission, err := a.Permissions.GetPermission(r.Context(), apppermissions.GetRequester{PermissionID: p[1]})
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, http.StatusOK, mapper.PermissionResponse(permission))
}
