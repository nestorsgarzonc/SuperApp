package writer

import (
	"net/http"
	"superapp/auth/internal/domain"
	shared "superapp/auth/internal/presentation/http/shared"
)

func (a *Handlers) createPermission(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.RequireSystemPermission(w, r, "auth.roles.manage"); !ok {
		return
	}
	var v domain.Permission
	if !shared.Decode(w, r, &v) {
		return
	}
	if err := a.Service.CreatePermission(r.Context(), v); err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 201, v)
}
func (a *Handlers) permissionRoute(w http.ResponseWriter, r *http.Request, p []string) {
	permission := "auth.roles.manage"
	if r.Method == "GET" {
		permission = "auth.roles.read"
	}
	var ok bool
	if permission == "auth.roles.manage" {
		_, ok = a.RequireSystemPermission(w, r, permission)
	} else {
		_, ok = a.RequirePermission(w, r, permission)
	}
	if !ok {
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
	case "GET":
		v, e := a.Service.PermissionByID(r.Context(), pid)
		if e != nil {
			a.Fail(w, e)
			return
		}
		shared.WriteJSON(w, 200, v)
	case "PATCH", "PUT":
		current, e := a.Service.PermissionByID(r.Context(), pid)
		if e != nil {
			a.Fail(w, e)
			return
		}
		var in struct {
			Code        *string `json:"code"`
			Description *string `json:"description"`
		}
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
		if e := a.Service.UpdatePermission(r.Context(), *current); e != nil {
			a.Fail(w, e)
			return
		}
		shared.WriteJSON(w, 200, current)
	case "DELETE":
		a.Respond(w, a.Service.DeletePermission(r.Context(), pid), 204, nil)
	default:
		shared.MethodNotAllowed(w)
	}
}
