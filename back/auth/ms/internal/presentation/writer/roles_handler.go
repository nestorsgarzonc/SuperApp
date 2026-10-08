package writer

import (
	"net/http"
	"superapp/auth/internal/core/domain"
	appauthorization "superapp/auth/internal/core/usecases/authorization"
	approles "superapp/auth/internal/core/usecases/roles"
	"superapp/auth/internal/presentation/mapper"
	"superapp/auth/internal/presentation/reader"
	shared "superapp/auth/internal/presentation/shared"
)

func (a *Handlers) roleRoute(w http.ResponseWriter, r *http.Request, p []string) {
	_, tenant, ok := a.RequirePermissionWithTenant(w, r, "auth.roles.manage")
	if !ok {
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
		request, ok := reader.UpdateRoleRequest(w, r, rid, tenant)
		if !ok {
			return
		}
		updated, err := a.Roles.UpdateRole(r.Context(), request)
		if err != nil {
			a.Fail(w, err)
			return
		}
		shared.WriteJSON(w, 200, mapper.RoleResponse(updated))
		return
	}
	if len(p) == 2 && r.Method == "DELETE" {
		_, err := a.Roles.DeleteRole(r.Context(), approles.DeleteRequester{RoleID: rid, TenantID: tenant})
		a.Respond(w, err, 204, nil)
		return
	}
	if len(p) == 3 && p[2] == "permissions" && r.Method == "POST" {
		request, ok := reader.CreateRolePermissionRequest(w, r, rid, tenant)
		if !ok {
			return
		}
		if !shared.ValidUUID(request.PermissionID) {
			a.Fail(w, domain.ErrInvalidArgument)
			return
		}
		_, err := a.Authorization.GrantPermission(r.Context(), request)
		a.Respond(w, err, 201, map[string]string{"status": "granted"})
		return
	}
	if len(p) == 4 && p[2] == "permissions" && r.Method == "DELETE" {
		if !shared.ValidUUID(p[3]) {
			a.Fail(w, domain.ErrInvalidArgument)
			return
		}
		_, err := a.Authorization.RevokePermission(r.Context(), appauthorization.RolePermissionParams{
			RoleID: rid, PermissionID: p[3], TenantID: tenant,
		})
		a.Respond(w, err, 204, nil)
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
	var tenant *string
	if request.Scope == "SYSTEM" {
		id, ok = a.RequireSystemPermission(w, r, "auth.roles.manage")
		if ok {
			var err error
			tenant, err = a.Tenant(r, id.UserID)
			if err != nil {
				a.Fail(w, err)
				return
			}
		}
	} else {
		id, tenant, ok = a.RequirePermissionWithTenant(w, r, "auth.roles.manage")
	}
	if !ok {
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
	created, err := a.Roles.CreateRole(r.Context(), request)
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 201, mapper.RoleResponse(created))
}
