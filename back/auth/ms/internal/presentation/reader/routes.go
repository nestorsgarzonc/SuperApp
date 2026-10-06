package reader

import (
	"net/http"
	"strings"
	shared "superapp/auth/internal/presentation/shared"
)

type Handlers struct{ *shared.API }

type Routes struct {
	h        *Handlers
	sessions *SessionsHandler
}

func New(api *shared.API) *Routes { return &Routes{h: &Handlers{api}, sessions: &SessionsHandler{api}} }
func (r *Routes) Handle(w http.ResponseWriter, req *http.Request) bool {
	path := req.URL.Path
	if len(path) > len("/api/v1/") && path[:len("/api/v1/")] == "/api/v1/" {
		path = strings.TrimRight(path, "/")
	}
	switch {
	case path == "/.well-known/jwks.json":
		r.h.jwks(w, req)
	case strings.HasPrefix(path, "/api/v1/users/") && req.Method == http.MethodGet:
		r.h.userRoute(w, req, strings.Split(strings.Trim(path, "/"), "/")[2:])
	case strings.HasPrefix(path, "/api/v1/roles/") && req.Method == http.MethodGet:
		r.h.roleRoute(w, req, strings.Split(strings.Trim(path, "/"), "/")[2:])
	case strings.HasPrefix(path, "/api/v1/permissions/") && req.Method == http.MethodGet:
		r.h.permissionRoute(w, req, strings.Split(strings.Trim(path, "/"), "/")[2:])
	case path == "/api/v1/sessions":
		if req.Method != http.MethodGet {
			return false
		}
		r.sessions.listSessions(w, req)
	case path == "/api/v1/roles":
		if req.Method != http.MethodGet {
			return false
		}
		r.h.listRoles(w, req)
	case path == "/api/v1/permissions":
		if req.Method != http.MethodGet {
			return false
		}
		r.h.listPermissions(w, req)
	default:
		return false
	}
	return true
}
