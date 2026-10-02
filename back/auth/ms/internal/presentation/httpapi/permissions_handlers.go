package httpapi

import (
	"net/http"

	"superapp/auth/internal/domain"
)

func (a *API) listPermissions(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requirePermission(w, r, "auth.roles.read"); !ok {
		return
	}
	v, err := a.service.Permissions(r.Context())
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, v)
}

func (a *API) createPermission(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSystemPermission(w, r, "auth.roles.manage"); !ok {
		return
	}
	var v domain.Permission
	if !decode(w, r, &v) {
		return
	}
	if err := a.service.CreatePermission(r.Context(), v); err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 201, v)
}

func (a *API) permissionRoute(w http.ResponseWriter, r *http.Request, p []string) {
	permission := "auth.roles.manage"
	if r.Method == "GET" {
		permission = "auth.roles.read"
	}
	var ok bool
	if permission == "auth.roles.manage" {
		_, ok = a.requireSystemPermission(w, r, permission)
	} else {
		_, ok = a.requirePermission(w, r, permission)
	}
	if !ok {
		return
	}
	if len(p) != 2 {
		writeError(w, 404, "not_found", "Route not found")
		return
	}
	pid := p[1]
	if !validUUID(pid) {
		a.fail(w, domain.ErrInvalidArgument)
		return
	}
	switch r.Method {
	case "GET":
		v, e := a.service.PermissionByID(r.Context(), pid)
		if e != nil {
			a.fail(w, e)
			return
		}
		writeJSON(w, 200, v)
	case "PATCH", "PUT":
		current, e := a.service.PermissionByID(r.Context(), pid)
		if e != nil {
			a.fail(w, e)
			return
		}
		var in struct {
			Code        *string `json:"code"`
			Description *string `json:"description"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.Code != nil {
			current.Code = *in.Code
		}
		if in.Description != nil {
			current.Description = *in.Description
		}
		if in.Code == nil && in.Description == nil {
			a.fail(w, domain.ErrInvalidArgument)
			return
		}
		if e := a.service.UpdatePermission(r.Context(), *current); e != nil {
			a.fail(w, e)
			return
		}
		writeJSON(w, 200, current)
	case "DELETE":
		a.respond(w, a.service.DeletePermission(r.Context(), pid), 204, nil)
	default:
		methodNotAllowed(w)
	}
}
