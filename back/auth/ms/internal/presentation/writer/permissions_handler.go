package writer

import (
	"net/http"
	"superapp/auth/internal/core/domain"
	apppermissions "superapp/auth/internal/core/usecases/permissions"
	"superapp/auth/internal/presentation/mapper"
	"superapp/auth/internal/presentation/reader"
	shared "superapp/auth/internal/presentation/shared"
)

func (a *Handlers) createPermission(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.RequireSystemPermission(w, r, "auth.roles.manage"); !ok {
		return
	}
	request, ok := reader.CreatePermissionRequest(w, r)
	if !ok {
		return
	}
	created, err := a.Permissions.CreatePermission(r.Context(), request)
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 201, mapper.PermissionResponse(created))
}
func (a *Handlers) permissionRoute(w http.ResponseWriter, r *http.Request, p []string) {
	if _, ok := a.RequireSystemPermission(w, r, "auth.roles.manage"); !ok {
		return
	}
	if len(p) != 2 {
		shared.WriteError(w, 404, "not_found", "Route not found")
		return
	}
	pid := p[1]
	if !shared.ValidUUID(pid) {
		a.Fail(w, domain.ErrInvalidArgument)
		return
	}
	switch r.Method {
	case "PATCH", "PUT":
		request, ok := reader.UpdatePermissionRequest(w, r, pid)
		if !ok {
			return
		}
		updated, err := a.Permissions.UpdatePermission(r.Context(), request)
		if err != nil {
			a.Fail(w, err)
			return
		}
		shared.WriteJSON(w, 200, mapper.PermissionResponse(updated))
	case "DELETE":
		_, err := a.Permissions.DeletePermission(r.Context(), apppermissions.DeleteRequester{PermissionID: pid})
		a.Respond(w, err, 204, nil)
	default:
		shared.MethodNotAllowed(w)
	}
}
