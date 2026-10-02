package writer

import (
	"net/http"
	"superapp/auth/internal/core/domain"
	dtoRoles "superapp/auth/internal/presentation/dto/roles"
	"superapp/auth/internal/presentation/mapper"
	"superapp/auth/internal/presentation/reader"
	shared "superapp/auth/internal/presentation/shared"
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
	allowed, err := a.Authorization.HasPermission(r.Context(), id.UserID, "auth.roles.manage", tenant)
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
	if len(p) == 2 && (r.Method == "PATCH" || r.Method == "PUT") {
		current, err := a.Roles.RoleByID(r.Context(), rid, tenant)
		if err != nil {
			a.Fail(w, err)
			return
		}
		var in dtoRoles.UpdateRequestDTO
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
		if err := a.Roles.UpdateRole(r.Context(), *current, tenant); err != nil {
			a.Fail(w, err)
			return
		}
		shared.WriteJSON(w, 200, mapper.RoleResponse(current))
		return
	}
	if len(p) == 2 && r.Method == "DELETE" {
		a.Respond(w, a.Roles.DeleteRole(r.Context(), rid, tenant), 204, nil)
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
		a.Respond(w, a.Authorization.GrantPermission(r.Context(), rid, in.PermissionID, tenant), 201, map[string]string{"status": "granted"})
		return
	}
	if len(p) == 4 && p[2] == "permissions" && r.Method == "DELETE" {
		if !shared.ValidUUID(p[3]) {
			a.Fail(w, domain.ErrInvalidArgument)
			return
		}
		a.Respond(w, a.Authorization.RevokePermission(r.Context(), rid, p[3], tenant), 204, nil)
		return
	}
	shared.WriteError(w, 404, "not_found", "Route not found")
}
func (a *Handlers) createRole(w http.ResponseWriter, r *http.Request) {
	request, ok := reader.CreateRoleRequest(w, r)
	if !ok {
		return
	}
	var id shared.Identity
	if request.Scope == "SYSTEM" {
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
	if request.Scope == "TENANT" {
		if tenant == nil {
			shared.WriteError(w, 403, "tenant_required", "Tenant context required")
			return
		}
		request.TenantID = tenant
	} else {
		request.TenantID = nil
	}
	created, err := a.CreateRole.CreateRoleFromRequest(r.Context(), request)
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 201, mapper.CreatedRoleResponse(created))
}
