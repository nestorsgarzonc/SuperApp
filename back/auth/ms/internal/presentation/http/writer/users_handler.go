package writer

import (
	"net/http"
	"strings"
	"superapp/auth/internal/domain"
	shared "superapp/auth/internal/presentation/http/shared"
)

func (a *Handlers) createUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.RequireSystemPermission(w, r, "auth.users.manage"); !ok {
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		FullName string `json:"full_name"`
	}
	if !shared.Decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Email) == "" || strings.TrimSpace(in.FullName) == "" {
		a.Fail(w, domain.ErrInvalidArgument)
		return
	}
	u, err := a.Service.Register(r.Context(), in.Email, in.Password, in.FullName)
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 201, u)
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
	if target == "me" && len(p) == 2 && r.Method == "GET" {
		u, err := a.Service.User(r.Context(), id.UserID)
		if err != nil {
			a.Fail(w, err)
			return
		}
		shared.WriteJSON(w, 200, u)
		return
	}
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
	if len(p) == 2 && r.Method == "GET" {
		u, err := a.Service.User(r.Context(), target)
		if err != nil {
			a.Fail(w, err)
			return
		}
		shared.WriteJSON(w, 200, u)
		return
	}
	if len(p) == 2 && (r.Method == "PATCH" || r.Method == "PUT") {
		if _, ok = a.RequireSystemPermission(w, r, "auth.users.manage"); !ok {
			return
		}
		var in struct {
			Email    *string `json:"email"`
			FullName *string `json:"full_name"`
			Status   *string `json:"status"`
		}
		if !shared.Decode(w, r, &in) {
			return
		}
		if in.Email != nil || in.FullName != nil {
			current, err := a.Service.User(r.Context(), target)
			if err != nil {
				a.Fail(w, err)
				return
			}
			email, name := current.Email, current.FullName
			if in.Email != nil {
				email = *in.Email
			}
			if in.FullName != nil {
				name = *in.FullName
			}
			if err := a.Service.UpdateUser(r.Context(), target, email, name); err != nil {
				a.Fail(w, err)
				return
			}
		}
		if in.Status != nil {
			if err := a.Service.SetUserStatus(r.Context(), target, *in.Status); err != nil {
				a.Fail(w, err)
				return
			}
		}
		if in.Email == nil && in.FullName == nil && in.Status == nil {
			a.Fail(w, domain.ErrInvalidArgument)
			return
		}
		w.WriteHeader(204)
		return
	}
	if len(p) == 3 && p[2] == "roles" && r.Method == "GET" {
		v, err := a.Service.EffectiveRoles(r.Context(), target, tenant)
		if err != nil {
			a.Fail(w, err)
			return
		}
		shared.WriteJSON(w, 200, v)
		return
	}
	if len(p) == 3 && p[2] == "permissions" && r.Method == "GET" {
		v, err := a.Service.EffectivePermissions(r.Context(), target, tenant)
		if err != nil {
			a.Fail(w, err)
			return
		}
		shared.WriteJSON(w, 200, v)
		return
	}
	if len(p) == 3 && p[2] == "roles" && r.Method == "POST" {
		if _, ok = a.RequireSystemPermission(w, r, "auth.users.manage"); !ok {
			return
		}
		var in struct {
			RoleID string `json:"role_id"`
		}
		if !shared.Decode(w, r, &in) {
			return
		}
		if !shared.ValidUUID(in.RoleID) {
			a.Fail(w, domain.ErrInvalidArgument)
			return
		}
		if tenant != nil {
			if err := a.TenantMemberAssertion(r, id.UserID, target, *tenant); err != nil {
				a.Fail(w, err)
				return
			}
		}
		a.Respond(w, a.Service.AssignRole(r.Context(), target, in.RoleID, tenant, id.UserID), 201, map[string]string{"status": "assigned"})
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
		a.Respond(w, a.Service.RevokeRole(r.Context(), target, p[3], tenant), 204, nil)
		return
	}
	shared.WriteError(w, 404, "not_found", "Route not found")
}
