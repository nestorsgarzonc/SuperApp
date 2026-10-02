package writer

import (
	"net/http"
	"superapp/auth/internal/core/domain"
	dtoPermissions "superapp/auth/internal/presentation/dto/permissions"
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
	created, err := a.CreatePermission.CreatePermissionFromRequest(r.Context(), request)
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 201, mapper.CreatedPermissionResponse(created))
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
		current, e := a.Permissions.PermissionByID(r.Context(), pid)
		if e != nil {
			a.Fail(w, e)
			return
		}
		var in dtoPermissions.UpdateRequestDTO
		if !shared.Decode(w, r, &in) {
			return
		}
		if in.Code != nil {
			current.Code = *in.Code
		}
		if in.Description != nil {
			current.Description = *in.Description
		}
		if in.Code == nil && in.Description == nil {
			a.Fail(w, domain.ErrInvalidArgument)
			return
		}
		if e := a.Permissions.UpdatePermission(r.Context(), *current); e != nil {
			a.Fail(w, e)
			return
		}
		shared.WriteJSON(w, 200, mapper.PermissionResponse(current))
	case "DELETE":
		a.Respond(w, a.Permissions.DeletePermission(r.Context(), pid), 204, nil)
	default:
		shared.MethodNotAllowed(w)
	}
}
