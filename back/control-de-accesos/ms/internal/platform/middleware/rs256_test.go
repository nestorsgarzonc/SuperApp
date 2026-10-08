package middleware

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

const (
	testIssuer   = "http://auth.test"
	testAudience = "superapp-api"
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func signRS256(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	h, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": kid})
	p, _ := json.Marshal(claims)
	msg := b64(h) + "." + b64(p)
	sum := sha256.Sum256([]byte(msg))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return msg + "." + b64(sig)
}

func setupRS256(t *testing.T, key *rsa.PrivateKey, kid string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": kid,
			"n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(srv.Close)
	if err := ConfigureRS256(RS256Config{JWKSURL: srv.URL, Issuer: testIssuer, Audience: testAudience}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { verifierMu.Lock(); verifier = nil; verifierMu.Unlock(); roleResolver = nil })
}

func goodClaims(user string) map[string]any {
	now := time.Now()
	return map[string]any{"iss": testIssuer, "aud": []string{testAudience}, "sub": user, "sid": "s1",
		"iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(time.Minute).Unix()}
}

func TestRS256Validation(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	setupRS256(t, key, "k1")
	user := uuid.NewString()

	mut := func(f func(map[string]any)) map[string]any { c := goodClaims(user); f(c); return c }
	cases := []struct {
		name  string
		token string
		ok    bool
	}{
		{"valid", signRS256(t, key, "k1", goodClaims(user)), true},
		{"wrong issuer", signRS256(t, key, "k1", mut(func(c map[string]any) { c["iss"] = "http://evil" })), false},
		{"wrong audience", signRS256(t, key, "k1", mut(func(c map[string]any) { c["aud"] = []string{"other"} })), false},
		{"expired", signRS256(t, key, "k1", mut(func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() })), false},
		{"no exp", signRS256(t, key, "k1", mut(func(c map[string]any) { delete(c, "exp") })), false},
		{"subject not uuid", signRS256(t, key, "k1", mut(func(c map[string]any) { c["sub"] = "admin" })), false},
		{"signed by another key", signRS256(t, other, "k1", goodClaims(user)), false},
		{"unknown kid", signRS256(t, key, "zzz", goodClaims(user)), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := ValidateJWTContext(context.Background(), tc.token)
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v, want ok=%v", err, tc.ok)
			}
			if tc.ok && (c.UserID != user || !c.fromJWKS) {
				t.Fatalf("claims=%+v", c)
			}
		})
	}
}

func TestRS256Disabled(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	if _, err := ValidateJWTContext(context.Background(), signRS256(t, key, "k1", goodClaims(uuid.NewString()))); err == nil {
		t.Fatal("RS256 must be rejected when not configured")
	}
}

func TestRS256ComplexAndRole(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	setupRS256(t, key, "k1")
	user, member, stranger := uuid.New(), uuid.New(), uuid.New()
	roleResolver = func(_ context.Context, u, c uuid.UUID) (string, bool, error) {
		if u == user && c == member {
			return "guard", true, nil
		}
		return "", false, nil
	}
	token := signRS256(t, key, "k1", goodClaims(user.String()))

	cases := []struct {
		name     string
		complex  string
		want     int
		wantRole string
	}{
		{"member", member.String(), 200, "GUARD"},
		{"not a member", stranger.String(), 403, ""},
		{"bad complex id", "x", 400, ""},
		{"no complex", "", 200, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, s := gatewayHandler(AuthMiddleware)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			if tc.complex != "" {
				req.Header.Set("X-Complex-ID", tc.complex)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d, want %d", rec.Code, tc.want)
			}
			if tc.want == 200 && (s.userID != user.String() || s.role != tc.wantRole) {
				t.Fatalf("seen=%+v", s)
			}
		})
	}
}
