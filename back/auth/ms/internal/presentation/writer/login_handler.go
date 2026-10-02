package writer

import (
	"errors"
	"net"
	"net/http"
	"superapp/auth/internal/core/domain"
	dtoAuth "superapp/auth/internal/presentation/dto/auth"
	dtoUsers "superapp/auth/internal/presentation/dto/users"
	shared "superapp/auth/internal/presentation/shared"
)

func (a *Handlers) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Device   string `json:"device_name"`
	}
	if !shared.Decode(w, r, &in) {
		return
	}
	if len(in.Device) > 128 {
		a.Fail(w, domain.ErrInvalidArgument)
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	u, p, err := a.Authentication.Login(r.Context(), in.Email, in.Password, in.Device, r.UserAgent(), ip)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			shared.WriteError(w, 401, "invalid_credentials", "Invalid email or password")
			return
		}
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 200, map[string]any{"user": dtoUsers.FromDomain(u), "tokens": dtoAuth.FromDomain(p)})
}
