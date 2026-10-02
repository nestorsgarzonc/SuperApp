package gatewayauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	userID    = uuid.NewString()
	complexID = uuid.NewString()
)

func serve(secret string, headers map[string]string, extra ...func(http.Handler) http.Handler) (int, *Identity) {
	var seen *Identity
	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := FromContext(r.Context())
		seen = &id
		w.WriteHeader(http.StatusOK)
	})
	for i := len(extra) - 1; i >= 0; i-- {
		h = extra[i](h)
	}
	h = Middleware(secret)(h)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, seen
}

func TestMiddleware_LoadsIdentity(t *testing.T) {
	code, id := serve("s3cret", map[string]string{
		HeaderSecret: "s3cret", HeaderUserID: userID, HeaderComplexID: complexID,
	})

	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, userID, id.UserID.String())
	assert.Equal(t, complexID, id.ComplexID.String())
}

func TestMiddleware_Rejections(t *testing.T) {
	cases := map[string]struct {
		headers map[string]string
		want    int
	}{
		"missing secret":    {map[string]string{HeaderUserID: userID}, http.StatusUnauthorized},
		"wrong secret":      {map[string]string{HeaderSecret: "nope", HeaderUserID: userID}, http.StatusUnauthorized},
		"missing user":      {map[string]string{HeaderSecret: "s3cret"}, http.StatusUnauthorized},
		"malformed user":    {map[string]string{HeaderSecret: "s3cret", HeaderUserID: "abc"}, http.StatusUnauthorized},
		"malformed complex": {map[string]string{HeaderSecret: "s3cret", HeaderUserID: userID, HeaderComplexID: "abc"}, http.StatusUnprocessableEntity},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			code, id := serve("s3cret", tc.headers)
			assert.Equal(t, tc.want, code)
			assert.Nil(t, id, "handler must not run")
		})
	}
}

func TestMiddleware_EmptySecretSkipsCheck(t *testing.T) {
	code, _ := serve("", map[string]string{HeaderUserID: userID})
	assert.Equal(t, http.StatusOK, code)
}

func TestRequireComplex(t *testing.T) {
	code, _ := serve("", map[string]string{HeaderUserID: userID}, RequireComplex)
	assert.Equal(t, http.StatusUnprocessableEntity, code)

	code, _ = serve("", map[string]string{HeaderUserID: userID, HeaderComplexID: complexID}, RequireComplex)
	assert.Equal(t, http.StatusOK, code)
}
