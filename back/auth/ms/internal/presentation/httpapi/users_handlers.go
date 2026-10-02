package httpapi

import (
	"net/http"
	"strings"

	"superapp/auth/internal/domain"
)

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSystemPermission(w, r, "auth.users.manage"); !ok {
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		FullName string `json:"full_name"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Email) == "" || strings.TrimSpace(in.FullName) == "" {
		a.fail(w, domain.ErrInvalidArgument)
		return
	}
	u, err := a.service.Register(r.Context(), in.Email, in.Password, in.FullName)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 201, u)
}

func (a *API) userRoute(w http.ResponseWriter, r *http.Request, p []string) {
	if len(p) < 2 {
		writeError(w, 404, "not_found", "Route not found")
		return
	}
	id, ok := a.auth(w, r)
	if !ok {
		return
	}
	target := p[1]
	if target == "me" && len(p) == 2 && r.Method == "GET" {
		u, err := a.service.User(r.Context(), id.UserID)
		if err != nil {
			a.fail(w, err)
			return
		}
		writeJSON(w, 200, u)
		return
	}
	if target == "me" {
		target = id.UserID
	}
	if !validUUID(target) {
		a.fail(w, domain.ErrInvalidArgument)
		return
	}
	tenant, terr := a.tenant(r, id.UserID)
	if terr != nil {
		a.fail(w, terr)
		return
	}
	if target != id.UserID {
		if _, ok = a.requireSystemPermission(w, r, "auth.users.read"); !ok {
			return
		}
	}
	if len(p) == 2 && r.Method == "GET" {
		u, err := a.service.User(r.Context(), target)
		if err != nil {
			a.fail(w, err)
			return
		}
		writeJSON(w, 200, u)
		return
	}
	if len(p) == 2 && (r.Method == "PATCH" || r.Method == "PUT") {
		if _, ok = a.requireSystemPermission(w, r, "auth.users.manage"); !ok {
			return
		}
		var in struct {
			Email    *string `json:"email"`
			FullName *string `json:"full_name"`
			Status   *string `json:"status"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.Email != nil || in.FullName != nil {
			current, err := a.service.User(r.Context(), target)
			if err != nil {
				a.fail(w, err)
				return
			}
			email, name := current.Email, current.FullName
			if in.Email != nil {
				email = *in.Email
			}
			if in.FullName != nil {
				name = *in.FullName
			}
			if err := a.service.UpdateUser(r.Context(), target, email, name); err != nil {
				a.fail(w, err)
				return
			}
		}
		if in.Status != nil {
			if err := a.service.SetUserStatus(r.Context(), target, *in.Status); err != nil {
				a.fail(w, err)
				return
			}
		}
		if in.Email == nil && in.FullName == nil && in.Status == nil {
			a.fail(w, domain.ErrInvalidArgument)
			return
		}
		w.WriteHeader(204)
		return
	}
	if len(p) == 3 && p[2] == "roles" && r.Method == "GET" {
		v, err := a.service.EffectiveRoles(r.Context(), target, tenant)
		if err != nil {
			a.fail(w, err)
			return
		}
		writeJSON(w, 200, v)
		return
	}
	if len(p) == 3 && p[2] == "permissions" && r.Method == "GET" {
		v, err := a.service.EffectivePermissions(r.Context(), target, tenant)
		if err != nil {
			a.fail(w, err)
			return
		}
		writeJSON(w, 200, v)
		return
	}
	if len(p) == 3 && p[2] == "roles" && r.Method == "POST" {
		if _, ok = a.requireSystemPermission(w, r, "auth.users.manage"); !ok {
			return
		}
		var in struct {
			RoleID string `json:"role_id"`
		}
		if !decode(w, r, &in) {
			return
		}
		if !validUUID(in.RoleID) {
			a.fail(w, domain.ErrInvalidArgument)
			return
		}
		if tenant != nil {
			if err := a.tenantMemberAssertion(r, id.UserID, target, *tenant); err != nil {
				a.fail(w, err)
				return
			}
		}
		a.respond(w, a.service.AssignRole(r.Context(), target, in.RoleID, tenant, id.UserID), 201, map[string]string{"status": "assigned"})
		return
	}
	if len(p) == 4 && p[2] == "roles" && r.Method == "DELETE" {
		if _, ok = a.requireSystemPermission(w, r, "auth.users.manage"); !ok {
			return
		}
		if !validUUID(p[3]) {
			a.fail(w, domain.ErrInvalidArgument)
			return
		}
		a.respond(w, a.service.RevokeRole(r.Context(), target, p[3], tenant), 204, nil)
		return
	}
	writeError(w, 404, "not_found", "Route not found")
}
