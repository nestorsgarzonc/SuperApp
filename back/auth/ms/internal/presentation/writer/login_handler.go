package writer

import (
	"errors"
	"net"
	"net/http"
	"superapp/auth/internal/core/domain"
	dtoAuth "superapp/auth/internal/presentation/dto/auth"
	"superapp/auth/internal/presentation/mapper"
	"superapp/auth/internal/presentation/reader"
	shared "superapp/auth/internal/presentation/shared"
)

func (a *Handlers) login(w http.ResponseWriter, r *http.Request) {
	request, ok := reader.LoginRequest(w, r)
	if !ok {
		return
	}
	if len(request.Device) > 128 {
		a.Fail(w, domain.ErrInvalidArgument)
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	request.IP = ip
	result, err := a.Login.Execute(r.Context(), request)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			shared.WriteError(w, 401, "invalid_credentials", "Invalid email or password")
			return
		}
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 200, dtoAuth.LoginResponseDTO{User: mapper.UserResponse(result.User), Tokens: mapper.TokenPairResponse(result.Tokens)})
}
