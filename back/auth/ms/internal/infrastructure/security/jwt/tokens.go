package jwt

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"superapp/auth/internal/core/ports/security"
)

type claims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}
type Tokens struct {
	private               *rsa.PrivateKey
	public                *rsa.PublicKey
	kid, issuer, audience string
	ttl                   time.Duration
}

func LoadTokens(path, issuer, audience string, ttl time.Duration) (*Tokens, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("JWT_PRIVATE_KEY_FILE must contain a PEM RSA private key")
	}
	var key *rsa.PrivateKey
	if parsed, e := x509.ParsePKCS1PrivateKey(block.Bytes); e == nil {
		key = parsed
	} else if k, e := x509.ParsePKCS8PrivateKey(block.Bytes); e == nil {
		var ok bool
		key, ok = k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("JWT private key is not RSA")
		}
	} else {
		return nil, errors.New("invalid RSA private key")
	}
	if key.N.BitLen() < 2048 {
		return nil, errors.New("JWT RSA key must be at least 2048 bits")
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	fingerprint := sha256.Sum256(pubDER)
	return &Tokens{private: key, public: &key.PublicKey, kid: base64.RawURLEncoding.EncodeToString(fingerprint[:12]), issuer: issuer, audience: audience, ttl: ttl}, nil
}
func (t *Tokens) Issue(userID, sessionID string) (string, time.Time, error) {
	now := time.Now()
	expires := now.Add(t.ttl)
	c := claims{SessionID: sessionID, RegisteredClaims: jwt.RegisteredClaims{Issuer: t.issuer, Subject: userID, Audience: jwt.ClaimStrings{t.audience}, IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires), ID: sessionID}}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	token.Header["kid"] = t.kid
	raw, err := token.SignedString(t.private)
	return raw, expires, err
}
func (t *Tokens) Verify(raw string) (*security.TokenClaims, error) {
	c := &claims{}
	token, err := jwt.ParseWithClaims(raw, c, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodRS256 {
			return nil, errors.New("unexpected signing method")
		}
		if token.Header["kid"] != t.kid {
			return nil, errors.New("unknown signing key")
		}
		return t.public, nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(t.issuer), jwt.WithAudience(t.audience), jwt.WithExpirationRequired())
	if err != nil || token == nil || !token.Valid || c.Subject == "" || c.SessionID == "" {
		return nil, errors.New("invalid or expired access token")
	}
	return &security.TokenClaims{Subject: c.Subject, Email: "", SessionID: c.SessionID, ExpiresAt: c.ExpiresAt.Time}, nil
}

type jwk struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (t *Tokens) JWKS() map[string]any {
	e := big.NewInt(int64(t.public.E)).Bytes()
	return map[string]any{"keys": []jwk{{Kty: "RSA", Use: "sig", Alg: "RS256", Kid: t.kid, N: base64.RawURLEncoding.EncodeToString(t.public.N.Bytes()), E: base64.RawURLEncoding.EncodeToString(e)}}}
}
func (t *Tokens) Discovery(baseURL string) map[string]any {
	return map[string]any{"issuer": t.issuer, "jwks_uri": baseURL + "/.well-known/jwks.json", "response_types_supported": []string{"token"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}, "claims_supported": []string{"iss", "sub", "aud", "exp", "iat", "sid"}}
}
