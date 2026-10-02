package writer

import (
	"net/http"
	"superapp/auth/internal/domain"
	shared "superapp/auth/internal/presentation/http/shared"
)

func (a *Handlers) checkPermission(w http.ResponseWriter, r *http.Request) {
	id, ok := a.Auth(w, r)
	if !ok {
		return
	}
	var in struct {
		UserID     string `json:"user_id"`
		Permission string `json:"permission"`
	}
	if !shared.Decode(w, r, &in) {
		return
	}
	if in.Permission == "" || !domain.ValidPermissionCode(in.Permission) {
		a.Fail(w, domain.ErrInvalidArgument)
		return
	}
	if in.UserID != "" && in.UserID != id.UserID {
		if !shared.ValidUUID(in.UserID) {
			a.Fail(w, domain.ErrInvalidArgument)
			return
		}
		if _, ok = a.RequirePermission(w, r, "auth.users.read"); !ok {
			return
		}
	} else {
		in.UserID = id.UserID
	}
	tenant, err := a.Tenant(r, id.UserID)
	if err != nil {
		a.Fail(w, err)
		return
	}
	allowed, err := a.Service.HasPermission(r.Context(), in.UserID, in.Permission, tenant)
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 200, map[string]bool{"allowed": allowed})
}
