package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func serve(method string, headers map[string]string) (*httptest.ResponseRecorder, bool) {
	reached := false
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusUnauthorized)
	}))
	req := httptest.NewRequest(method, "/users/me/notifications", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, reached
}

func TestPreflightAnsweredWithoutAuth(t *testing.T) {
	rec, reached := serve(http.MethodOptions, map[string]string{
		"Origin": "https://app.example.com", "Access-Control-Request-Method": "GET",
	})
	assert.False(t, reached)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "X-Complex-ID")
}

func TestResponsesCarryOrigin(t *testing.T) {
	rec, reached := serve(http.MethodGet, map[string]string{"Origin": "https://app.example.com"})
	assert.True(t, reached)
	assert.Equal(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"))

	rec, _ = serve(http.MethodGet, nil)
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"), "no Origin, no CORS headers")
}
