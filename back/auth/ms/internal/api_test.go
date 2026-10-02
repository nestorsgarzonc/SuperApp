package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/usecases/auth/login"
	shared "superapp/auth/internal/presentation/shared"
)

type loginFailureUseCase struct{}

func (loginFailureUseCase) Execute(context.Context, login.Requester) (login.Presenter, error) {
	return login.Presenter{}, domain.ErrInvalidCredentials
}

func TestInvalidLoginDoesNotRevealAddress(t *testing.T) {
	services := shared.Services{Login: loginFailureUseCase{}}
	api := New(services, nil)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"known@example.test","password":"wrong"}`))
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "known@example.test") {
		t.Fatalf("response leaked email: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Invalid email or password") {
		t.Fatalf("unexpected generic response: %s", w.Body.String())
	}
}
