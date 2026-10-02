package httpapi

import (
	"errors"
	"net"
	"net/http"

	"superapp/auth/internal/domain"
)

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
