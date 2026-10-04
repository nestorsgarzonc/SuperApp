package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type fakeStaff struct {
	// key: userID|complexID|role
	staff map[string]bool
	err   error
}

func (f fakeStaff) IsStaffOfComplex(_ context.Context, u, c uuid.UUID, role string) (bool, error) {
	return f.staff[u.String()+"|"+c.String()+"|"+role], f.err
}

func TestRequireStaffMiddleware(t *testing.T) {
	user, complex1, complex2 := uuid.New(), uuid.New(), uuid.New()
	checker := fakeStaff{staff: map[string]bool{
		user.String() + "|" + complex1.String() + "|GUARD": true,
	}}

	cases := []struct {
		name    string
		role    string
		complex uuid.UUID
		checker fakeStaff
		want    int
	}{
		{"guard of this complex", "GUARD", complex1, checker, http.StatusOK},
		{"guard claim but staff of another complex", "GUARD", complex2, checker, http.StatusForbidden},
		{"role claim not allowed", "RESIDENT", complex1, checker, http.StatusForbidden},
		{"admin claim but only guard in db", "ADMIN", complex1, checker, http.StatusForbidden},
		{"no role at all", "", complex1, checker, http.StatusForbidden},
		{"lookup failure fails closed", "GUARD", complex1, fakeStaff{err: errors.New("db down")}, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			h := RequireStaffMiddleware(tc.checker, "GUARD", "ADMIN")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
			}))

			ctx := context.WithValue(context.Background(), UserIDKey, user.String())
			ctx = context.WithValue(ctx, ComplexIDKey, tc.complex.String())
			if tc.role != "" {
				ctx = context.WithValue(ctx, UserRoleKey, tc.role)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx))

			if tc.want == http.StatusOK {
				if !called || rec.Code != http.StatusOK {
					t.Fatalf("expected handler to run, got %d called=%v", rec.Code, called)
				}
				return
			}
			if called || rec.Code != tc.want {
				t.Fatalf("expected %d without running handler, got %d called=%v", tc.want, rec.Code, called)
			}
		})
	}
}

func TestRequireStaffMiddlewareNeedsComplexInContext(t *testing.T) {
	h := RequireStaffMiddleware(fakeStaff{}, "GUARD")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not run")
	}))
	ctx := context.WithValue(context.Background(), UserIDKey, uuid.NewString())
	ctx = context.WithValue(ctx, UserRoleKey, "GUARD")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without a resolved complex, got %d", rec.Code)
	}
}

func TestMiddlewareErrorsUseTheSameEnvelopeAsHandlers(t *testing.T) {
	rec := httptest.NewRecorder()
	respondWithError(rec, http.StatusForbidden, "nope")
	body := rec.Body.String()
	for _, want := range []string{`"success":false`, `"code":"PERM_001"`, `"message":"nope"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("error body %s must contain %s", body, want)
		}
	}
	if rec.Code != http.StatusForbidden || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected response %d %v", rec.Code, rec.Header())
	}
}
