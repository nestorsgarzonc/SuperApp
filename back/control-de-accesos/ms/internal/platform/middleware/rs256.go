package middleware

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// RS256 access tokens are issued by the auth service and verified here against
// its public keys (JWKS). They carry only the identity (sub); the complex and
// the role are resolved from this service's own memberships, so the token never
// goes stale when a membership changes.

const (
	jwksRefreshMinInterval = 30 * time.Second
	jwksCacheTTL           = 10 * time.Minute
	jwtLeeway              = 5 * time.Second
	minRSAKeyBits          = 2048
)

// RS256Config enables verification of auth-issued tokens. An empty JWKSURL
// leaves RS256 disabled (tokens with alg RS256 are rejected).
type RS256Config struct {
	JWKSURL  string
	Issuer   string
	Audience string
}

type jwksVerifier struct {
	cfg    RS256Config
	client *http.Client

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
	lastTry   time.Time
}

var (
	verifierMu sync.RWMutex
	verifier   *jwksVerifier

	roleResolver ComplexRoleResolver
)

// ComplexRoleResolver returns the role a user holds in a complex (found=false
// when the user does not belong to it).
type ComplexRoleResolver func(ctx context.Context, userID, complexID uuid.UUID) (role string, found bool, err error)

// ConfigureRS256 enables RS256 verification. Issuer is required so a token
// signed by the right key but meant for another issuer is not accepted.
func ConfigureRS256(cfg RS256Config) error {
	if cfg.JWKSURL == "" {
		return nil
	}
	if cfg.Issuer == "" || cfg.Audience == "" {
		return errors.New("AUTH_JWT_ISSUER and AUTH_JWT_AUDIENCE are required with AUTH_JWKS_URL")
	}
	verifierMu.Lock()
	verifier = &jwksVerifier{cfg: cfg, client: &http.Client{Timeout: 3 * time.Second}}
	verifierMu.Unlock()
	return nil
}

// SetComplexRoleResolver wires the membership lookup used for RS256 tokens.
func SetComplexRoleResolver(r ComplexRoleResolver) { roleResolver = r }

func currentVerifier() *jwksVerifier {
	verifierMu.RLock()
	defer verifierMu.RUnlock()
	return verifier
}

type jwksDoc struct {
	Keys []struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Alg string `json:"alg"`
		Use string `json:"use"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

func (v *jwksVerifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.cfg.JWKSURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks: unexpected status %d", resp.StatusCode)
	}
	var doc jwksDoc
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return err
	}
	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" || (k.Use != "" && k.Use != "sig") {
			continue
		}
		nb, err1 := base64.RawURLEncoding.DecodeString(k.N)
		eb, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil {
			continue
		}
		n := new(big.Int).SetBytes(nb)
		e := new(big.Int).SetBytes(eb)
		if n.BitLen() < minRSAKeyBits || !e.IsInt64() || e.Int64() < 3 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: n, E: int(e.Int64())}
	}
	if len(keys) == 0 {
		return errors.New("jwks: no usable RSA keys")
	}
	v.mu.Lock()
	v.keys, v.fetchedAt = keys, time.Now()
	v.mu.Unlock()
	return nil
}

// key returns the public key for kid. An unknown kid (rotation) triggers one
// refresh, rate limited so a forged kid cannot make us hammer the auth service.
func (v *jwksVerifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	k, ok := v.keys[kid]
	fresh := time.Since(v.fetchedAt) < jwksCacheTTL
	v.mu.RUnlock()
	if ok && fresh {
		return k, nil
	}

	v.mu.Lock()
	if time.Since(v.lastTry) < jwksRefreshMinInterval && v.keys != nil {
		k, ok = v.keys[kid]
		v.mu.Unlock()
		if ok {
			return k, nil
		}
		return nil, errors.New("unknown signing key")
	}
	v.lastTry = time.Now()
	v.mu.Unlock()

	if err := v.refresh(ctx); err != nil {
		// Keep serving the cached key if the auth service is briefly down.
		if ok {
			return k, nil
		}
		return nil, fmt.Errorf("could not load signing keys: %w", err)
	}
	v.mu.RLock()
	k, ok = v.keys[kid]
	v.mu.RUnlock()
	if !ok {
		return nil, errors.New("unknown signing key")
	}
	return k, nil
}

type rs256Claims struct {
	Subject   string          `json:"sub"`
	Issuer    string          `json:"iss"`
	Audience  json.RawMessage `json:"aud"`
	ExpiresAt int64           `json:"exp"`
	NotBefore int64           `json:"nbf"`
	IssuedAt  int64           `json:"iat"`
}

func (c rs256Claims) hasAudience(want string) bool {
	var one string
	if json.Unmarshal(c.Audience, &one) == nil {
		return one == want
	}
	var many []string
	if json.Unmarshal(c.Audience, &many) == nil {
		for _, a := range many {
			if a == want {
				return true
			}
		}
	}
	return false
}

// validateRS256 verifies signature, issuer, audience and time bounds.
func validateRS256(ctx context.Context, parts []string, kid string) (*JWTClaims, error) {
	v := currentVerifier()
	if v == nil {
		return nil, errors.New("unsupported JWT algorithm")
	}
	if kid == "" {
		return nil, errors.New("missing key id")
	}
	pub, err := v.key(ctx, kid)
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("invalid signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		return nil, errors.New("invalid signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}
	var c rs256Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}
	if c.Issuer != v.cfg.Issuer || !c.hasAudience(v.cfg.Audience) {
		return nil, errors.New("invalid issuer or audience")
	}
	now := time.Now()
	if c.ExpiresAt == 0 || now.After(time.Unix(c.ExpiresAt, 0).Add(jwtLeeway)) {
		return nil, errors.New("token expired")
	}
	if c.NotBefore != 0 && now.Add(jwtLeeway).Before(time.Unix(c.NotBefore, 0)) {
		return nil, errors.New("token not yet valid")
	}
	userID, err := uuid.Parse(c.Subject)
	if err != nil {
		return nil, errors.New("invalid subject")
	}
	return &JWTClaims{UserID: userID.String(), IssuedAt: c.IssuedAt, ExpiresAt: c.ExpiresAt, fromJWKS: true}, nil
}

// attachComplex resolves the complex (X-Complex-ID) and the role of an
// RS256-authenticated user from the memberships. No header = no complex.
// A complex the user does not belong to is an error (403).
func attachComplex(r *http.Request, claims *JWTClaims) (status int, err error) {
	raw := strings.TrimSpace(r.Header.Get(HeaderComplexID))
	if raw == "" {
		return 0, nil
	}
	complexID, perr := uuid.Parse(raw)
	if perr != nil {
		return http.StatusBadRequest, errors.New("invalid complex ID format")
	}
	if roleResolver == nil {
		return http.StatusInternalServerError, errors.New("membership lookup not configured")
	}
	userID, _ := uuid.Parse(claims.UserID)
	role, found, rerr := roleResolver(r.Context(), userID, complexID)
	if rerr != nil {
		return http.StatusInternalServerError, errors.New("failed to verify complex access")
	}
	if !found {
		return http.StatusForbidden, errors.New("you do not have access to this complex")
	}
	claims.ComplexID = complexID.String()
	claims.Role = strings.ToUpper(strings.TrimSpace(role))
	return 0, nil
}
