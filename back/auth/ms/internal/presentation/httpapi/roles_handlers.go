package httpapi

import (
	"net/http"

	"superapp/auth/internal/domain"
)

func (a *API) roleRoute(w http.ResponseWriter, r *http.Request, p []string) {
	id, ok := a.auth(w, r)
	if !ok {
		return
	}
	tenant, err := a.tenant(r, id.UserID)
	if err != nil {
		a.fail(w, err)
		return
	}
	permission := "auth.roles.manage"
	if len(p) == 2 && r.Method == "GET" {
		permission = "auth.roles.read"
	}
	allowed, err := a.service.HasPermission(r.Context(), id.UserID, permission, tenant)
	if err != nil {
		a.fail(w, err)
		return
	}
	if !allowed {
		writeError(w, 403, "forbidden", "Permission denied")
		return
	}
	if len(p) < 2 {
		writeError(w, 404, "not_found", "Route not found")
		return
	}
	rid := p[1]
	if !validUUID(rid) {
		a.fail(w, domain.ErrInvalidArgument)
		return
	}
	if len(p) == 2 && r.Method == "GET" {
		v, err := a.service.RoleByID(r.Context(), rid, tenant)
		if err != nil {
			a.fail(w, err)
			return
		}
		writeJSON(w, 200, v)
		return
	}
	if len(p) == 2 && (r.Method == "PATCH" || r.Method == "PUT") {
		current, err := a.service.RoleByID(r.Context(), rid, tenant)
		if err != nil {
			a.fail(w, err)
			return
		}
		var in struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
			Enabled     *bool   `json:"enabled"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.Name != nil {
			current.Name = *in.Name
		}
		if in.Description != nil {
			current.Description = *in.Description
		}
		if in.Enabled != nil {
			current.Enabled = *in.Enabled
		}
		if in.Name == nil && in.Description == nil && in.Enabled == nil {
			a.fail(w, domain.ErrInvalidArgument)
			return
		}
		if err := a.service.UpdateRole(r.Context(), *current, tenant); err != nil {
			a.fail(w, err)
			return
		}
		writeJSON(w, 200, current)
		return
	}
	if len(p) == 2 && r.Method == "DELETE" {
		a.respond(w, a.service.DeleteRole(r.Context(), rid, tenant), 204, nil)
		return
	}
	if len(p) == 3 && p[2] == "permissions" && r.Method == "POST" {
		var in struct {
			PermissionID string `json:"permission_id"`
		}
		if !decode(w, r, &in) {
			return
		}
		if !validUUID(in.PermissionID) {
			a.fail(w, domain.ErrInvalidArgument)
			return
		}
		a.respond(w, a.service.GrantPermission(r.Context(), rid, in.PermissionID, tenant), 201, map[string]string{"status": "granted"})
		return
	}
	if len(p) == 4 && p[2] == "permissions" && r.Method == "DELETE" {
		if !validUUID(p[3]) {
			a.fail(w, domain.ErrInvalidArgument)
			return
		}
		a.respond(w, a.service.RevokePermission(r.Context(), rid, p[3], tenant), 204, nil)
		return
	}
	writeError(w, 404, "not_found", "Route not found")
}

func (a *API) listRoles(w http.ResponseWriter, r *http.Request) {
	id, ok := a.requirePermission(w, r, "auth.roles.read")
	if !ok {
		return
	}
	tenant, err := a.tenant(r, id.UserID)
	if err != nil {
		a.fail(w, err)
		return
	}
	v, err := a.service.Roles(r.Context(), tenant)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, v)
}

func (a *API) createRole(w http.ResponseWriter, r *http.Request) {
	var v domain.Role
	if !decode(w, r, &v) {
		return
	}
	var id identity
	var ok bool
	if v.Scope == "SYSTEM" {
		id, ok = a.requireSystemPermission(w, r, "auth.roles.manage")
	} else {
		id, ok = a.requirePermission(w, r, "auth.roles.manage")
	}
	if !ok {
		return
	}
	tenant, err := a.tenant(r, id.UserID)
	if err != nil {
		a.fail(w, err)
		return
	}
	if v.Scope == "TENANT" {
		if tenant == nil {
			writeError(w, 403, "tenant_required", "Tenant context required")
			return
		}
		v.TenantID = tenant
	} else {
		v.TenantID = nil
	}
	if err := a.service.CreateRole(r.Context(), v); err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 201, v)
}
