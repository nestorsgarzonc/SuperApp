package reader

import (
	"net/http"

	"superapp/auth/internal/core/domain"
	appauthorization "superapp/auth/internal/core/usecases/authorization"
	appusers "superapp/auth/internal/core/usecases/users"
	"superapp/auth/internal/presentation/mapper"
	shared "superapp/auth/internal/presentation/shared"
)

func (a *Handlers) userRoute(w http.ResponseWriter, r *http.Request, p []string) {
	if len(p) < 2 {
		shared.WriteError(w, http.StatusNotFound, "not_found", "Route not found")
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
	tenant, err := a.Tenant(r, id.UserID)
	if err != nil {
		a.Fail(w, err)
		return
	}
	if target != id.UserID {
		if _, ok = a.RequireSystemPermission(w, r, "auth.users.read"); !ok {
			return
		}
	}
	switch {
	case len(p) == 2:
		user, err := a.Users.GetUser(r.Context(), appusers.GetRequester{UserID: target})
		if err != nil {
			a.Fail(w, err)
			return
		}
		shared.WriteJSON(w, http.StatusOK, mapper.UserPresenterResponse(user))
	case len(p) == 3 && p[2] == "roles":
		roles, err := a.Authorization.EffectiveRoles(r.Context(), appauthorization.EffectiveRolesRequester{UserID: target, TenantID: tenant})
		if err != nil {
			a.Fail(w, err)
			return
		}
		shared.WriteJSON(w, http.StatusOK, mapper.AuthorizedRoleResponses(roles))
	case len(p) == 3 && p[2] == "permissions":
		permissions, err := a.Authorization.EffectivePermissions(r.Context(), appauthorization.EffectivePermissionsRequester{UserID: target, TenantID: tenant})
		if err != nil {
			a.Fail(w, err)
			return
		}
		shared.WriteJSON(w, http.StatusOK, mapper.EffectivePermissionsResponse(permissions))
	default:
		shared.WriteError(w, http.StatusNotFound, "not_found", "Route not found")
	}
}
