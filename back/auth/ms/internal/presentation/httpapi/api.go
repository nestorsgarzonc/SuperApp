package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"superapp/auth/internal/application"
	"superapp/auth/internal/domain"
)

type Service interface {
	Ready(context.Context) error
	Register(context.Context, string, string, string) (*domain.User, error)
	Login(context.Context, string, string, string, string, string) (*domain.User, *application.TokenPair, error)
	Refresh(context.Context, string) (*application.TokenPair, error)
	Logout(context.Context, string) error
	LogoutAll(context.Context, string) error
	Authenticate(string) (*application.TokenClaims, error)
	User(context.Context, string) (*domain.User, error)
	UpdateUser(context.Context, string, string, string) error
	Sessions(context.Context, string) ([]domain.Session, error)
	RevokeSession(context.Context, string, string) error
	HasPermission(context.Context, string, string, *string) (bool, error)
	EffectivePermissions(context.Context, string, *string) ([]string, error)
	EffectiveRoles(context.Context, string, *string) ([]domain.Role, error)
	Roles(context.Context, *string) ([]domain.Role, error)
	RoleByID(context.Context, string, *string) (*domain.Role, error)
	CreateRole(context.Context, domain.Role) error
	UpdateRole(context.Context, domain.Role, *string) error
	DeleteRole(context.Context, string, *string) error
	Permissions(context.Context) ([]domain.Permission, error)
	PermissionByID(context.Context, string) (*domain.Permission, error)
	CreatePermission(context.Context, domain.Permission) error
	UpdatePermission(context.Context, domain.Permission) error
	DeletePermission(context.Context, string) error
	GrantPermission(context.Context, string, string, *string) error
	RevokePermission(context.Context, string, string, *string) error
	AssignRole(context.Context, string, string, *string, string) error
	RevokeRole(context.Context, string, string, *string) error
	SetUserStatus(context.Context, string, string) error
}
type API struct {
	service      Service
	tenantSecret []byte
	mux          *http.ServeMux
}
type identity struct{ UserID, Email, SessionID string }

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func New(service Service, tenantSecret []byte) *API {
	a := &API{service: service, tenantSecret: tenantSecret, mux: http.NewServeMux()}
	a.mux.HandleFunc("/health", a.health)
	a.mux.HandleFunc("/ready", a.ready)
	a.mux.HandleFunc("/.well-known/jwks.json", a.jwks)
	a.mux.HandleFunc("/api/v1/", a.route)
	return a
}
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.mux.ServeHTTP(w, r) }
func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}
	if err := a.service.Ready(r.Context()); err != nil {
		writeError(w, 503, "not_ready", "Auth dependencies unavailable")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ready"})
}
func (a *API) jwks(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}
	if jwkProvider, ok := a.service.(interface{ JWKS() map[string]any }); ok {
		w.Header().Set("Cache-Control", "public, max-age=300")
		writeJSON(w, 200, jwkProvider.JWKS())
		return
	}
	writeError(w, 500, "jwks_unavailable", "Signing keys unavailable")
}
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
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Device   string `json:"device_name"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Device) > 128 {
		a.fail(w, domain.ErrInvalidArgument)
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	u, p, err := a.service.Login(r.Context(), in.Email, in.Password, in.Device, r.UserAgent(), ip)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			writeError(w, 401, "invalid_credentials", "Invalid email or password")
			return
		}
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"user": u, "tokens": p})
}
func (a *API) refresh(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.RefreshToken) > 512 {
		a.fail(w, domain.ErrInvalidToken)
		return
	}
	p, err := a.service.Refresh(r.Context(), in.RefreshToken)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, p)
}
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.RefreshToken) > 512 {
		w.WriteHeader(204)
		return
	}
	a.respond(w, a.service.Logout(r.Context(), in.RefreshToken), 204, nil)
}
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
func (a *API) roleRoute(w http.ResponseWriter, r *http.Request, p []string) {
	id, ok := a.auth(w, r)
	if !ok {
		return
	}
	tenant, err := a.tenant(r, id.UserID)
	if err != nil {
		a.fail(w, err)
		return
	}
	permission := "auth.roles.manage"
	if len(p) == 2 && r.Method == "GET" {
		permission = "auth.roles.read"
	}
	allowed, err := a.service.HasPermission(r.Context(), id.UserID, permission, tenant)
	if err != nil {
		a.fail(w, err)
		return
	}
	if !allowed {
		writeError(w, 403, "forbidden", "Permission denied")
		return
	}
	if len(p) < 2 {
		writeError(w, 404, "not_found", "Route not found")
		return
	}
	rid := p[1]
	if !validUUID(rid) {
		a.fail(w, domain.ErrInvalidArgument)
		return
	}
	if len(p) == 2 && r.Method == "GET" {
		v, err := a.service.RoleByID(r.Context(), rid, tenant)
		if err != nil {
			a.fail(w, err)
			return
		}
		writeJSON(w, 200, v)
		return
	}
	if len(p) == 2 && (r.Method == "PATCH" || r.Method == "PUT") {
		current, err := a.service.RoleByID(r.Context(), rid, tenant)
		if err != nil {
			a.fail(w, err)
			return
		}
		var in struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
			Enabled     *bool   `json:"enabled"`
		}
		if !decode(w, r, &in) {
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
			a.fail(w, domain.ErrInvalidArgument)
			return
		}
		if err := a.service.UpdateRole(r.Context(), *current, tenant); err != nil {
			a.fail(w, err)
			return
		}
		writeJSON(w, 200, current)
		return
	}
	if len(p) == 2 && r.Method == "DELETE" {
		a.respond(w, a.service.DeleteRole(r.Context(), rid, tenant), 204, nil)
		return
	}
	if len(p) == 3 && p[2] == "permissions" && r.Method == "POST" {
		var in struct {
			PermissionID string `json:"permission_id"`
		}
		if !decode(w, r, &in) {
			return
		}
		if !validUUID(in.PermissionID) {
			a.fail(w, domain.ErrInvalidArgument)
			return
		}
		a.respond(w, a.service.GrantPermission(r.Context(), rid, in.PermissionID, tenant), 201, map[string]string{"status": "granted"})
		return
	}
	if len(p) == 4 && p[2] == "permissions" && r.Method == "DELETE" {
		if !validUUID(p[3]) {
			a.fail(w, domain.ErrInvalidArgument)
			return
		}
		a.respond(w, a.service.RevokePermission(r.Context(), rid, p[3], tenant), 204, nil)
		return
	}
	writeError(w, 404, "not_found", "Route not found")
}
func (a *API) listRoles(w http.ResponseWriter, r *http.Request) {
	id, ok := a.requirePermission(w, r, "auth.roles.read")
	if !ok {
		return
	}
	tenant, err := a.tenant(r, id.UserID)
	if err != nil {
		a.fail(w, err)
		return
	}
	v, err := a.service.Roles(r.Context(), tenant)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (a *API) createRole(w http.ResponseWriter, r *http.Request) {
	var v domain.Role
	if !decode(w, r, &v) {
		return
	}
	var id identity
	var ok bool
	if v.Scope == "SYSTEM" {
		id, ok = a.requireSystemPermission(w, r, "auth.roles.manage")
	} else {
		id, ok = a.requirePermission(w, r, "auth.roles.manage")
	}
	if !ok {
		return
	}
	tenant, err := a.tenant(r, id.UserID)
	if err != nil {
		a.fail(w, err)
		return
	}
	if v.Scope == "TENANT" {
		if tenant == nil {
			writeError(w, 403, "tenant_required", "Tenant context required")
			return
		}
		v.TenantID = tenant
	} else {
		v.TenantID = nil
	}
	if err := a.service.CreateRole(r.Context(), v); err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 201, v)
}
func (a *API) listPermissions(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requirePermission(w, r, "auth.roles.read"); !ok {
		return
	}
	v, err := a.service.Permissions(r.Context())
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (a *API) createPermission(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSystemPermission(w, r, "auth.roles.manage"); !ok {
		return
	}
	var v domain.Permission
	if !decode(w, r, &v) {
		return
	}
	if err := a.service.CreatePermission(r.Context(), v); err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 201, v)
}
func (a *API) permissionRoute(w http.ResponseWriter, r *http.Request, p []string) {
	permission := "auth.roles.manage"
	if r.Method == "GET" {
		permission = "auth.roles.read"
	}
	var ok bool
	if permission == "auth.roles.manage" {
		_, ok = a.requireSystemPermission(w, r, permission)
	} else {
		_, ok = a.requirePermission(w, r, permission)
	}
	if !ok {
		return
	}
	if len(p) != 2 {
		writeError(w, 404, "not_found", "Route not found")
		return
	}
	pid := p[1]
	if !validUUID(pid) {
		a.fail(w, domain.ErrInvalidArgument)
		return
	}
	switch r.Method {
	case "GET":
		v, e := a.service.PermissionByID(r.Context(), pid)
		if e != nil {
			a.fail(w, e)
			return
		}
		writeJSON(w, 200, v)
	case "PATCH", "PUT":
		current, e := a.service.PermissionByID(r.Context(), pid)
		if e != nil {
			a.fail(w, e)
			return
		}
		var in struct {
			Code        *string `json:"code"`
			Description *string `json:"description"`
		}
		if !decode(w, r, &in) {
			return
		}
		if in.Code != nil {
			current.Code = *in.Code
		}
		if in.Description != nil {
			current.Description = *in.Description
		}
		if in.Code == nil && in.Description == nil {
			a.fail(w, domain.ErrInvalidArgument)
			return
		}
		if e := a.service.UpdatePermission(r.Context(), *current); e != nil {
			a.fail(w, e)
			return
		}
		writeJSON(w, 200, current)
	case "DELETE":
		a.respond(w, a.service.DeletePermission(r.Context(), pid), 204, nil)
	default:
		methodNotAllowed(w)
	}
}
func (a *API) checkPermission(w http.ResponseWriter, r *http.Request) {
	id, ok := a.auth(w, r)
	if !ok {
		return
	}
	var in struct {
		UserID     string `json:"user_id"`
		Permission string `json:"permission"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Permission == "" || !domain.ValidPermissionCode(in.Permission) {
		a.fail(w, domain.ErrInvalidArgument)
		return
	}
	if in.UserID != "" && in.UserID != id.UserID {
		if !validUUID(in.UserID) {
			a.fail(w, domain.ErrInvalidArgument)
			return
		}
		if _, ok = a.requirePermission(w, r, "auth.users.read"); !ok {
			return
		}
	} else {
		in.UserID = id.UserID
	}
	tenant, err := a.tenant(r, id.UserID)
	if err != nil {
		a.fail(w, err)
		return
	}
	allowed, err := a.service.HasPermission(r.Context(), in.UserID, in.Permission, tenant)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"allowed": allowed})
}
func (a *API) auth(w http.ResponseWriter, r *http.Request) (identity, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		writeError(w, 401, "unauthorized", "Bearer access token required")
		return identity{}, false
	}
	claims, err := a.service.Authenticate(strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		writeError(w, 401, "unauthorized", "Invalid or expired access token")
		return identity{}, false
	}
	return identity{claims.Subject, claims.Email, claims.SessionID}, true
}
func (a *API) requirePermission(w http.ResponseWriter, r *http.Request, permission string) (identity, bool) {
	id, ok := a.auth(w, r)
	if !ok {
		return identity{}, false
	}
	tenant, err := a.tenant(r, id.UserID)
	if err != nil {
		a.fail(w, err)
		return identity{}, false
	}
	allowed, err := a.service.HasPermission(r.Context(), id.UserID, permission, tenant)
	if err != nil {
		a.fail(w, err)
		return identity{}, false
	}
	if !allowed {
		writeError(w, 403, "forbidden", "Permission denied")
		return identity{}, false
	}
	return id, true
}
func (a *API) requireSystemPermission(w http.ResponseWriter, r *http.Request, permission string) (identity, bool) {
	id, ok := a.auth(w, r)
	if !ok {
		return identity{}, false
	}
	allowed, err := a.service.HasPermission(r.Context(), id.UserID, permission, nil)
	if err != nil {
		a.fail(w, err)
		return identity{}, false
	}
	if !allowed {
		writeError(w, 403, "forbidden", "Permission denied")
		return identity{}, false
	}
	return id, true
}
func (a *API) tenant(r *http.Request, userID string) (*string, error) {
	value := strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
	if value == "" {
		return nil, nil
	}
	if !validUUID(value) {
		return nil, domain.ErrForbidden
	}
	if len(a.tenantSecret) < 32 {
		return nil, domain.ErrForbidden
	}
	sig := r.Header.Get("X-Gateway-Signature")
	timestamp := r.Header.Get("X-Gateway-Timestamp")
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(unix, 0)) > time.Minute || time.Until(time.Unix(unix, 0)) > time.Minute {
		return nil, domain.ErrForbidden
	}
	mac := hmac.New(sha256.New, a.tenantSecret)
	_, _ = mac.Write([]byte(userID + "\n" + value + "\n" + timestamp))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.ToLower(sig)), []byte(expected)) {
		return nil, domain.ErrForbidden
	}
	return &value, nil
}

// tenantMemberAssertion validates a Gateway assertion that the target user, not
// merely the caller, belongs to the requested tenant.
func (a *API) tenantMemberAssertion(r *http.Request, actorID, targetID, tenantID string) error {
	timestamp := r.Header.Get("X-Gateway-Membership-Timestamp")
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(unix, 0)) > time.Minute || time.Until(time.Unix(unix, 0)) > time.Minute {
		return domain.ErrForbidden
	}
	mac := hmac.New(sha256.New, a.tenantSecret)
	_, _ = mac.Write([]byte(actorID + "\n" + targetID + "\n" + tenantID + "\n" + timestamp))
	expected := hex.EncodeToString(mac.Sum(nil))
	provided := strings.ToLower(r.Header.Get("X-Gateway-Membership-Signature"))
	if len(a.tenantSecret) < 32 || !hmac.Equal([]byte(provided), []byte(expected)) {
		return domain.ErrForbidden
	}
	return nil
}
func (a *API) respond(w http.ResponseWriter, err error, status int, payload any) {
	if err != nil {
		a.fail(w, err)
		return
	}
	if status == 204 {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, payload)
}
func (a *API) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, 403, "forbidden", "Permission denied")
	case errors.Is(err, domain.ErrInvalidCredentials), errors.Is(err, domain.ErrInvalidToken):
		writeError(w, 401, "unauthorized", "Authentication failed")
	case errors.Is(err, domain.ErrAccountDisabled):
		writeError(w, 403, "account_disabled", "Account is not active")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, 404, "not_found", "Resource not found")
	case errors.Is(err, domain.ErrEmailTaken), errors.Is(err, domain.ErrConflict):
		writeError(w, 409, "conflict", "Resource conflict")
	case errors.Is(err, domain.ErrInvalidArgument):
		writeError(w, 400, "invalid_request", err.Error())
	default:
		writeError(w, 500, "internal_error", "Internal server error")
	}
}
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, 400, "invalid_json", "Invalid request body")
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, 400, "invalid_json", "Request must contain one JSON value")
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, 405, "method_not_allowed", "Method not allowed")
}
func validUUID(value string) bool { return uuidPattern.MatchString(value) }
