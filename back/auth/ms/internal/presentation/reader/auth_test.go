package reader

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthRequestsMapJSONToRequesters(t *testing.T) {
	t.Run("login", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"user@example.com","password":"secret","device_name":"laptop"}`))
		r.Header.Set("User-Agent", "test-client")
		request, ok := LoginRequest(httptest.NewRecorder(), r)
		if !ok || request.Email != "user@example.com" || request.Password != "secret" || request.Device != "laptop" || request.UserAgent != "test-client" {
			t.Fatalf("unexpected login requester: %+v, ok=%t", request, ok)
		}
	})

	t.Run("register", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"user@example.com","password":"secret","full_name":"Test User"}`))
		request, ok := RegisterRequest(httptest.NewRecorder(), r)
		if !ok || request.Email != "user@example.com" || request.Password != "secret" || request.Name != "Test User" {
			t.Fatalf("unexpected register requester: %+v, ok=%t", request, ok)
		}
	})

	t.Run("refresh", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"refresh_token":"refresh-value"}`))
		request, ok := RefreshRequest(httptest.NewRecorder(), r)
		if !ok || request.RefreshToken != "refresh-value" {
			t.Fatalf("unexpected refresh requester: %+v, ok=%t", request, ok)
		}
	})

	t.Run("logout", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"refresh_token":"refresh-value"}`))
		request, ok := LogoutRequest(httptest.NewRecorder(), r)
		if !ok || request.RefreshToken != "refresh-value" {
			t.Fatalf("unexpected logout requester: %+v, ok=%t", request, ok)
		}
	})
}
