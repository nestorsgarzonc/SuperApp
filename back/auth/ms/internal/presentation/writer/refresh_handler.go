package writer

import (
	"net/http"
	"superapp/auth/internal/core/domain"
	dtoAuth "superapp/auth/internal/presentation/dto/auth"
	"superapp/auth/internal/presentation/mapper"
	"superapp/auth/internal/presentation/reader"
	shared "superapp/auth/internal/presentation/shared"
)

func (a *Handlers) refresh(w http.ResponseWriter, r *http.Request) {
	request, ok := reader.RefreshRequest(w, r)
	if !ok {
		return
	}
	if len(request.RefreshToken) > 512 {
		a.Fail(w, domain.ErrInvalidToken)
		return
	}
	result, err := a.Refresh.Execute(r.Context(), request)
	if err != nil {
		a.Fail(w, err)
		return
	}
	shared.WriteJSON(w, 200, dtoAuth.RefreshResponseDTO(mapper.TokenPairResponse(result.Tokens)))
}
