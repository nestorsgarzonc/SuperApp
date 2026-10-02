package httpapi

import (
	"net/http"
	"strings"

	"superapp/auth/internal/domain"
)

func (a *API) route(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/" {
		writeError(w, 404, "not_found", "Route not found")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/")
	ctx := r.Context()
	switch {
	case path == "auth/login" && r.Method == "POST":
		a.login(w, r)
	case path == "auth/refresh" && r.Method == "POST":
		a.refresh(w, r)
	case path == "auth/logout" && r.Method == "POST":
		a.logout(w, r)
	case path == "auth/logout-all" && r.Method == "POST":
		if id, ok := a.auth(w, r); ok {
			a.respond(w, a.service.LogoutAll(ctx, id.UserID), 204, nil)
		}
	case path == "users" && r.Method == "POST":
		a.createUser(w, r)
	case path == "sessions" && r.Method == "GET":
		if id, ok := a.auth(w, r); ok {
			v, err := a.service.Sessions(ctx, id.UserID)
			if err != nil {
				a.fail(w, err)
			} else {
				writeJSON(w, 200, v)
			}
		}
	case strings.HasPrefix(path, "sessions/") && r.Method == "DELETE":
		if id, ok := a.auth(w, r); ok {
			sessionID := strings.TrimPrefix(path, "sessions/")
			if !validUUID(sessionID) {
				a.fail(w, domain.ErrInvalidArgument)
				return
			}
			a.respond(w, a.service.RevokeSession(ctx, id.UserID, sessionID), 204, nil)
		}
	case strings.HasPrefix(path, "users/"):
		a.userRoute(w, r, strings.Split(path, "/"))
	case path == "roles" && r.Method == "GET":
		a.listRoles(w, r)
	case path == "roles" && r.Method == "POST":
		a.createRole(w, r)
	case strings.HasPrefix(path, "roles/"):
		a.roleRoute(w, r, strings.Split(path, "/"))
	case path == "permissions" && r.Method == "GET":
		a.listPermissions(w, r)
	case path == "permissions" && r.Method == "POST":
		a.createPermission(w, r)
	case strings.HasPrefix(path, "permissions/"):
		a.permissionRoute(w, r, strings.Split(path, "/"))
	case path == "auth/check-permission" && r.Method == "POST":
		a.checkPermission(w, r)
	default:
		if r.Method != "GET" && r.Method != "POST" && r.Method != "PATCH" && r.Method != "PUT" && r.Method != "DELETE" {
			methodNotAllowed(w)
		} else {
			writeError(w, 404, "not_found", "Route not found")
		}
	}
}
