package tokenverify

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	user   = "10000000-0000-0000-0000-000000000001"
	issuer = "http://auth:8081"
	aud    = "superapp-api"
	hsKey  = "dev-secret"
)

func setup(t *testing.T) (*Verifier, *rsa.PrivateKey) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "kid": "k1",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(srv.Close)
	v, err := New(Config{JWKSURL: srv.URL, Issuer: issuer, Audience: aud, HS256Key: hsKey})
	require.NoError(t, err)
	return v, key
}

func rs(t *testing.T, key *rsa.PrivateKey, c jwt.MapClaims) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	tok.Header["kid"] = "k1"
	raw, err := tok.SignedString(key)
	require.NoError(t, err)
	return raw
}

func hs(t *testing.T, secret string, c jwt.MapClaims) string {
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(secret))
	require.NoError(t, err)
	return raw
}

func TestSubject_Valid(t *testing.T) {
	v, key := setup(t)
	exp := time.Now().Add(time.Hour).Unix()
	for name, raw := range map[string]string{
		"auth RS256":      rs(t, key, jwt.MapClaims{"iss": issuer, "aud": aud, "sub": user, "exp": exp}),
		"dev-login HS256": hs(t, hsKey, jwt.MapClaims{"user_id": user, "exp": exp}),
	} {
		id, err := v.Subject(context.Background(), raw)
		require.NoError(t, err, name)
		assert.Equal(t, user, id.String(), name)
	}
}

func TestSubject_Rejected(t *testing.T) {
	v, key := setup(t)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	exp := time.Now().Add(time.Hour).Unix()
	cases := map[string]string{
		"forged RS256":    rs(t, other, jwt.MapClaims{"iss": issuer, "aud": aud, "sub": user, "exp": exp}),
		"wrong issuer":    rs(t, key, jwt.MapClaims{"iss": "evil", "aud": aud, "sub": user, "exp": exp}),
		"wrong audience":  rs(t, key, jwt.MapClaims{"iss": issuer, "aud": "other", "sub": user, "exp": exp}),
		"expired":         rs(t, key, jwt.MapClaims{"iss": issuer, "aud": aud, "sub": user, "exp": time.Now().Add(-time.Hour).Unix()}),
		"no exp":          rs(t, key, jwt.MapClaims{"iss": issuer, "aud": aud, "sub": user}),
		"bad subject":     rs(t, key, jwt.MapClaims{"iss": issuer, "aud": aud, "sub": "abc", "exp": exp}),
		"HS256 wrong key": hs(t, "guess", jwt.MapClaims{"user_id": user, "exp": exp}),
		"alg none":        "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"`+user+`"}`)) + ".",
		"unsigned junk":   "x." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"`+user+`"}`)) + ".sig",
	}
	for name, raw := range cases {
		_, err := v.Subject(context.Background(), raw)
		assert.ErrorIs(t, err, ErrInvalidToken, name)
	}
}

func TestSubject_OnlyConfiguredAlgorithms(t *testing.T) {
	v, err := New(Config{HS256Key: hsKey})
	require.NoError(t, err)
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	_, err = v.Subject(context.Background(), rs(t, key, jwt.MapClaims{"iss": issuer, "aud": aud, "sub": user, "exp": time.Now().Add(time.Hour).Unix()}))
	assert.ErrorIs(t, err, ErrInvalidToken, "RS256 accepted without JWKS")

	_, err = New(Config{})
	assert.Error(t, err, "a verifier with no keys must not be created")
}
