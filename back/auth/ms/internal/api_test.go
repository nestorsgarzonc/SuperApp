package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/ports/security"
	"superapp/auth/internal/core/usecases/contracts"
	shared "superapp/auth/internal/presentation/shared"
)

type loginFailureService struct{}

func (loginFailureService) Register(context.Context, string, string, string) (*domain.User, error) {
	return nil, nil
}
func (loginFailureService) Login(context.Context, string, string, string, string, string) (*domain.User, *contracts.TokenPair, error) {
	return nil, nil, domain.ErrInvalidCredentials
}
func (loginFailureService) Refresh(context.Context, string) (*contracts.TokenPair, error) {
	return nil, nil
}
func (loginFailureService) Logout(context.Context, string) error { return nil }
func (loginFailureService) Authenticate(string) (*security.TokenClaims, error) {
	return nil, nil
}
func (loginFailureService) JWKS() map[string]any { return nil }

func TestInvalidLoginDoesNotRevealAddress(t *testing.T) {
	services := shared.Services{Auth: loginFailureService{}}
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
