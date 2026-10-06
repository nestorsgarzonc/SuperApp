package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type seen struct {
	called                  bool
	userID, complexID, role string
}

func gatewayHandler(mw func(http.Handler) http.Handler) (http.Handler, *seen) {
	s := &seen{}
	return mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.called = true
		s.userID = GetUserID(r.Context())
		s.complexID = GetComplexID(r.Context())
		s.role = GetUserRole(r.Context())
	})), s
}

func TestGatewayIdentity(t *testing.T) {
	t.Setenv("GATEWAY_SECRET", "gw-secret")
	t.Setenv("JWT_SECRET", "jwt-secret")
	user, cx := uuid.NewString(), uuid.NewString()

	cases := []struct {
		name       string
		headers    map[string]string
		want       int
		wantUser   string
		wantCx     string
		wantRole   string
		wantCalled bool
	}{
		{"valid gateway request", map[string]string{"X-Gateway-Secret": "gw-secret", "X-User-ID": user, "X-Complex-ID": cx, "X-Complex-Role": "guard"}, 200, user, cx, "GUARD", true},
		{"valid without complex", map[string]string{"X-Gateway-Secret": "gw-secret", "X-User-ID": user}, 200, user, "", "", true},
		{"wrong secret", map[string]string{"X-Gateway-Secret": "nope", "X-User-ID": user}, 401, "", "", "", false},
		{"missing user", map[string]string{"X-Gateway-Secret": "gw-secret"}, 401, "", "", "", false},
		{"user not a uuid", map[string]string{"X-Gateway-Secret": "gw-secret", "X-User-ID": "admin"}, 401, "", "", "", false},
		{"complex not a uuid", map[string]string{"X-Gateway-Secret": "gw-secret", "X-User-ID": user, "X-Complex-ID": "x"}, 401, "", "", "", false},
		// Identity headers without the secret are just client input: no identity, no JWT -> 401.
		{"spoofed identity without secret", map[string]string{"X-User-ID": user, "X-Complex-Role": "ADMIN"}, 401, "", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, s := gatewayHandler(AuthMiddleware)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tc.want || s.called != tc.wantCalled {
				t.Fatalf("status=%d called=%v, want %d/%v", rec.Code, s.called, tc.want, tc.wantCalled)
			}
			if s.userID != tc.wantUser || s.complexID != tc.wantCx || s.role != tc.wantRole {
				t.Fatalf("identity = %+v", *s)
			}
		})
	}
}

func TestGatewayRejectedWhenNoSecretIsConfigured(t *testing.T) {
	t.Setenv("GATEWAY_SECRET", "")
	h, s := gatewayHandler(AuthMiddleware)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Gateway-Secret", "")
	req.Header.Set("X-User-ID", uuid.NewString())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || s.called {
		t.Fatalf("an empty configured secret must never authenticate anyone, got %d", rec.Code)
	}
}

func TestGatewayRequestIgnoredByJWTPathAndJWTStillWorks(t *testing.T) {
	t.Setenv("GATEWAY_SECRET", "gw-secret")
	t.Setenv("JWT_SECRET", "jwt-secret")
	user := uuid.NewString()
	token, err := GenerateJWT(user, "a@b.c", "RESIDENT", uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}

	h, s := gatewayHandler(AuthMiddleware)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 || s.userID != user || s.role != "RESIDENT" {
		t.Fatalf("JWT auth must keep working next to the gateway path: %d %+v", rec.Code, *s)
	}
}

func TestOptionalAuthHonoursGatewayIdentity(t *testing.T) {
	t.Setenv("GATEWAY_SECRET", "gw-secret")
	user := uuid.NewString()
	h, s := gatewayHandler(OptionalAuthMiddleware)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Gateway-Secret", "gw-secret")
	req.Header.Set("X-User-ID", user)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || s.userID != user {
		t.Fatalf("got %d %+v", rec.Code, *s)
	}

	bad := httptest.NewRequest(http.MethodGet, "/", nil)
	bad.Header.Set("X-Gateway-Secret", "wrong")
	bad.Header.Set("X-User-ID", user)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, bad)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("a wrong gateway secret must be rejected even in development, got %d", rec.Code)
	}
}
