package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func serve(method string, headers map[string]string) (*httptest.ResponseRecorder, bool) {
	reached := false
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(method, "/api/v1/auth/login", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, reached
}

func TestPreflightAnsweredBeforeRouter(t *testing.T) {
	rec, reached := serve(http.MethodOptions, map[string]string{
		"Origin": "https://nestorsgarzonc.github.io", "Access-Control-Request-Method": "POST",
	})
	if reached || rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("code=%d reached=%v headers=%v", rec.Code, reached, rec.Header())
	}
}

func TestRequestAllowsAnyOriginWithoutCredentials(t *testing.T) {
	rec, reached := serve(http.MethodPost, map[string]string{"Origin": "https://anything.example"})
	if !reached || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("reached=%v headers=%v", reached, rec.Header())
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("credentials must not be allowed together with *")
	}
}

func TestNoOriginUntouched(t *testing.T) {
	rec, _ := serve(http.MethodPost, nil)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unexpected CORS header")
	}
}
