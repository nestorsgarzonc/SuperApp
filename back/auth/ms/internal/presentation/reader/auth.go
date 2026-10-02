package reader

import (
	"net/http"

	"superapp/auth/internal/core/usecases/auth/login"
	"superapp/auth/internal/core/usecases/auth/logout"
	"superapp/auth/internal/core/usecases/auth/refresh"
	"superapp/auth/internal/core/usecases/auth/register"
	dtoAuth "superapp/auth/internal/presentation/dto/auth"
	shared "superapp/auth/internal/presentation/shared"
)

func LoginRequest(w http.ResponseWriter, r *http.Request) (login.Requester, bool) {
	var dto dtoAuth.LoginRequestDTO
	if !shared.Decode(w, r, &dto) {
		return login.Requester{}, false
	}
	return login.Requester{Email: dto.Email, Password: dto.Password, Device: dto.Device, UserAgent: r.UserAgent()}, true
}

func RegisterRequest(w http.ResponseWriter, r *http.Request) (register.Requester, bool) {
	var dto dtoAuth.RegisterRequestDTO
	if !shared.Decode(w, r, &dto) {
		return register.Requester{}, false
	}
	return register.Requester{Email: dto.Email, Password: dto.Password, Name: dto.FullName}, true
}

func RefreshRequest(w http.ResponseWriter, r *http.Request) (refresh.Requester, bool) {
	var dto dtoAuth.RefreshRequestDTO
	if !shared.Decode(w, r, &dto) {
		return refresh.Requester{}, false
	}
	return refresh.Requester{RefreshToken: dto.RefreshToken}, true
}

func LogoutRequest(w http.ResponseWriter, r *http.Request) (logout.Requester, bool) {
	var dto dtoAuth.LogoutRequestDTO
	if !shared.Decode(w, r, &dto) {
		return logout.Requester{}, false
	}
	return logout.Requester{RefreshToken: dto.RefreshToken}, true
}
