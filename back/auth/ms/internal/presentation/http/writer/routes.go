package writer

import (
	"net/http"
	"strings"
	shared "superapp/auth/internal/presentation/http/shared"
)

type Handlers struct{ *shared.API }

type Routes struct {
	h        *Handlers
	sessions *SessionsHandler
}

func New(api *shared.API) *Routes { return &Routes{h: &Handlers{api}, sessions: &SessionsHandler{api}} }
func (r *Routes) Handle(w http.ResponseWriter, req *http.Request) bool {
	p := req.URL.Path
	if len(p) > len("/api/v1/") && p[:len("/api/v1/")] == "/api/v1/" {
		p = strings.TrimRight(p, "/")
	}
	switch {
	case p == "/api/v1/auth/login" && req.Method == http.MethodPost:
		r.h.login(w, req)
	case p == "/api/v1/auth/refresh" && req.Method == http.MethodPost:
		r.h.refresh(w, req)
	case p == "/api/v1/auth/logout" && req.Method == http.MethodPost:
		r.h.logout(w, req)
	case p == "/api/v1/auth/logout-all" && req.Method == http.MethodPost:
		r.sessions.logoutAll(w, req)
	case p == "/api/v1/auth/check-permission" && req.Method == http.MethodPost:
		r.h.checkPermission(w, req)
	case p == "/api/v1/users" && req.Method == http.MethodPost:
		r.h.createUser(w, req)
	case strings.HasPrefix(p, "/api/v1/sessions/") && req.Method == http.MethodDelete:
		r.sessions.revokeSession(w, req)
	case strings.HasPrefix(p, "/api/v1/users/"):
		r.h.userRoute(w, req, strings.Split(strings.Trim(p, "/"), "/")[2:])
	case p == "/api/v1/roles" && req.Method == http.MethodPost:
		r.h.createRole(w, req)
	case strings.HasPrefix(p, "/api/v1/roles/"):
		r.h.roleRoute(w, req, strings.Split(strings.Trim(p, "/"), "/")[2:])
	case p == "/api/v1/permissions" && req.Method == http.MethodPost:
		r.h.createPermission(w, req)
	case strings.HasPrefix(p, "/api/v1/permissions/"):
		r.h.permissionRoute(w, req, strings.Split(strings.Trim(p, "/"), "/")[2:])
	default:
		return false
	}
	return true
}
