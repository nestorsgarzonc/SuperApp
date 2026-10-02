package writer

import (
	"net/http"
	"superapp/auth/internal/core/domain"
	appauthorization "superapp/auth/internal/core/usecases/authorization"
	dtoAuth "superapp/auth/internal/presentation/dto/auth"
	"superapp/auth/internal/presentation/mapper"
	"superapp/auth/internal/presentation/reader"
	shared "superapp/auth/internal/presentation/shared"
)

func (a *Handlers) createUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.RequireSystemPermission(w, r, "auth.users.manage"); !ok {
		return
	}
	request, ok := reader.RegisterRequest(w, r)
	if !ok {
		return
	}
	result, err := a.Register.Execute(r.Context(), request)
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 201, dtoAuth.RegisterResponseDTO(mapper.AuthUserResponse(result.User)))
}
func (a *Handlers) userRoute(w http.ResponseWriter, r *http.Request, p []string) {
	if len(p) < 2 {
		shared.WriteError(w, 404, "not_found", "Route not found")
		return
	}
	id, ok := a.Auth(w, r)
	if !ok {
		return
	}
	target := p[1]
	if target == "me" {
		target = id.UserID
	}
	if !shared.ValidUUID(target) {
		a.Fail(w, domain.ErrInvalidArgument)
		return
	}
	tenant, terr := a.Tenant(r, id.UserID)
	if terr != nil {
		a.Fail(w, terr)
		return
	}
	if target != id.UserID {
		if _, ok = a.RequireSystemPermission(w, r, "auth.users.read"); !ok {
			return
		}
	}
	if len(p) == 2 && (r.Method == "PATCH" || r.Method == "PUT") {
		if _, ok = a.RequireSystemPermission(w, r, "auth.users.manage"); !ok {
			return
		}
		request, ok := reader.UpdateUserRequest(w, r, target)
		if !ok {
			return
		}
		if _, err := a.Users.UpdateUser(r.Context(), request); err != nil {
			a.Fail(w, err)
			return
		}
		w.WriteHeader(204)
		return
	}
	if len(p) == 3 && p[2] == "roles" && r.Method == "POST" {
		if _, ok = a.RequireSystemPermission(w, r, "auth.users.manage"); !ok {
			return
		}
		request, ok := reader.AssignRoleRequest(w, r, target, tenant, id.UserID)
		if !ok {
			return
		}
		if !shared.ValidUUID(request.RoleID) {
			a.Fail(w, domain.ErrInvalidArgument)
			return
		}
		if tenant != nil {
			if err := a.TenantMemberAssertion(r, shared.TenantMemberAssertionParams{
				ActorID: id.UserID, TargetID: target, TenantID: *tenant,
			}); err != nil {
				a.Fail(w, err)
				return
			}
		}
		_, err := a.Authorization.AssignRole(r.Context(), request)
		a.Respond(w, err, 201, map[string]string{"status": "assigned"})
		return
	}
	if len(p) == 4 && p[2] == "roles" && r.Method == "DELETE" {
		if _, ok = a.RequireSystemPermission(w, r, "auth.users.manage"); !ok {
			return
		}
		if !shared.ValidUUID(p[3]) {
			a.Fail(w, domain.ErrInvalidArgument)
			return
		}
		_, err := a.Authorization.RevokeRole(r.Context(), appauthorization.RevokeRoleParams{
			UserID: target, RoleID: p[3], TenantID: tenant,
		})
		a.Respond(w, err, 204, nil)
		return
	}
	shared.WriteError(w, 404, "not_found", "Route not found")
}
