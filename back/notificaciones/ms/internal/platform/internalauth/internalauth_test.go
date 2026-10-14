package internalauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func serve(secret, header string) int {
	h := Middleware(secret)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/internal/notifications", nil)
	if header != "" {
		req.Header.Set(HeaderSecret, header)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestMiddleware(t *testing.T) {
	assert.Equal(t, http.StatusNoContent, serve("s3cret", "s3cret"))
	assert.Equal(t, http.StatusUnauthorized, serve("s3cret", ""))
	assert.Equal(t, http.StatusUnauthorized, serve("s3cret", "wrong"))
	assert.Equal(t, http.StatusNoContent, serve("", ""), "empty secret = development")
}
