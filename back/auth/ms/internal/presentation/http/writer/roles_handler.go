package writer

import (
	"net/http"
	"superapp/auth/internal/domain"
	shared "superapp/auth/internal/presentation/http/shared"
)

func (a *Handlers) roleRoute(w http.ResponseWriter, r *http.Request, p []string) {
	id, ok := a.Auth(w, r)
	if !ok {
		return
	}
	tenant, err := a.Tenant(r, id.UserID)
	if err != nil {
		a.Fail(w, err)
		return
	}
	permission := "auth.roles.manage"
	if len(p) == 2 && r.Method == "GET" {
		permission = "auth.roles.read"
	}
	allowed, err := a.Service.HasPermission(r.Context(), id.UserID, permission, tenant)
	if err != nil {
		a.Fail(w, err)
		return
	}
	if !allowed {
		shared.WriteError(w, 403, "forbidden", "Permission denied")
		return
	}
	if len(p) < 2 {
		shared.WriteError(w, 404, "not_found", "Route not found")
		return
	}
	rid := p[1]
	if !shared.ValidUUID(rid) {
		a.Fail(w, domain.ErrInvalidArgument)
		return
	}
	if len(p) == 2 && r.Method == "GET" {
		v, err := a.Service.RoleByID(r.Context(), rid, tenant)
		if err != nil {
			a.Fail(w, err)
			return
		}
		shared.WriteJSON(w, 200, v)
		return
	}
	if len(p) == 2 && (r.Method == "PATCH" || r.Method == "PUT") {
		current, err := a.Service.RoleByID(r.Context(), rid, tenant)
		if err != nil {
			a.Fail(w, err)
			return
		}
		var in struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
			Enabled     *bool   `json:"enabled"`
		}
		if !shared.Decode(w, r, &in) {
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
			a.Fail(w, domain.ErrInvalidArgument)
			return
		}
		if err := a.Service.UpdateRole(r.Context(), *current, tenant); err != nil {
			a.Fail(w, err)
			return
		}
		shared.WriteJSON(w, 200, current)
		return
	}
	if len(p) == 2 && r.Method == "DELETE" {
		a.Respond(w, a.Service.DeleteRole(r.Context(), rid, tenant), 204, nil)
		return
	}
	if len(p) == 3 && p[2] == "permissions" && r.Method == "POST" {
		var in struct {
			PermissionID string `json:"permission_id"`
		}
		if !shared.Decode(w, r, &in) {
			return
		}
		if !shared.ValidUUID(in.PermissionID) {
			a.Fail(w, domain.ErrInvalidArgument)
			return
		}
		a.Respond(w, a.Service.GrantPermission(r.Context(), rid, in.PermissionID, tenant), 201, map[string]string{"status": "granted"})
		return
	}
	if len(p) == 4 && p[2] == "permissions" && r.Method == "DELETE" {
		if !shared.ValidUUID(p[3]) {
			a.Fail(w, domain.ErrInvalidArgument)
			return
		}
		a.Respond(w, a.Service.RevokePermission(r.Context(), rid, p[3], tenant), 204, nil)
		return
	}
	shared.WriteError(w, 404, "not_found", "Route not found")
}
func (a *Handlers) createRole(w http.ResponseWriter, r *http.Request) {
	var v domain.Role
	if !shared.Decode(w, r, &v) {
		return
	}
	var id shared.Identity
	var ok bool
	if v.Scope == "SYSTEM" {
		id, ok = a.RequireSystemPermission(w, r, "auth.roles.manage")
	} else {
		id, ok = a.RequirePermission(w, r, "auth.roles.manage")
	}
	if !ok {
		return
	}
	tenant, err := a.Tenant(r, id.UserID)
	if err != nil {
		a.Fail(w, err)
		return
	}
	if v.Scope == "TENANT" {
		if tenant == nil {
			shared.WriteError(w, 403, "tenant_required", "Tenant context required")
			return
		}
		v.TenantID = tenant
	} else {
		v.TenantID = nil
	}
	if err := a.Service.CreateRole(r.Context(), v); err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 201, v)
}
