// Package authn verifies the access tokens the front sends.
//
// Production tokens are RS256, issued by the auth service and verified locally
// with its public keys (JWKS), so the gateway never calls auth per request.
// In development the gateway may also accept HS256 tokens minted by
// control-de-accesos' /dev/token (DEV_HS256_SECRET), which is what the
// /dev-login page of the front uses.
package authn

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	jwksCacheTTL           = 10 * time.Minute
	jwksRefreshMinInterval = 30 * time.Second
	leeway                 = 5 * time.Second
	minRSAKeyBits          = 2048
)

var ErrInvalidToken = errors.New("invalid token")

// Identity is what the gateway learns from a valid token.
type Identity struct {
	UserID string
	// SessionID is the auth session ("sid" claim) of an RS256 token; it is
	// what auth marks as revoked on logout.
	SessionID string
	// DevToken marks an HS256 token from the dev tooling.
	DevToken bool
}

type Config struct {
	JWKSURL  string
	Issuer   string
	Audience string
	// DevHS256Secret enables HS256 dev tokens. Empty = only RS256.
	DevHS256Secret string
	HTTPClient     *http.Client
}

type Verifier struct {
	cfg  Config
	jwks *jwks
	rs   *jwt.Parser
	hs   *jwt.Parser
}

func NewVerifier(cfg Config) *Verifier {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 3 * time.Second}
	}
	return &Verifier{
		cfg:  cfg,
		jwks: &jwks{url: cfg.JWKSURL, client: cfg.HTTPClient},
		rs: jwt.NewParser(
			jwt.WithValidMethods([]string{"RS256"}),
			jwt.WithIssuer(cfg.Issuer),
			jwt.WithAudience(cfg.Audience),
			jwt.WithExpirationRequired(),
			jwt.WithIssuedAt(),
			jwt.WithLeeway(leeway),
		),
		hs: jwt.NewParser(
			jwt.WithValidMethods([]string{"HS256"}),
			jwt.WithExpirationRequired(),
			jwt.WithLeeway(leeway),
		),
	}
}

// Warm loads the signing keys ahead of the first request. Failure is not
// fatal: auth may still be starting, and keys load lazily on first use.
func (v *Verifier) Warm(ctx context.Context) error { return v.jwks.refresh(ctx) }

// Ready reports whether signing keys have been loaded at least once.
func (v *Verifier) Ready() bool { return v.jwks.loaded() }

func (v *Verifier) Verify(ctx context.Context, raw string) (Identity, error) {
	// The algorithm decides which key material and which claim rules apply;
	// each parser only accepts its own algorithm, which closes alg confusion.
	unverified, _, err := jwt.NewParser().ParseUnverified(raw, jwt.MapClaims{})
	if err != nil {
		return Identity{}, ErrInvalidToken
	}
	switch unverified.Method.Alg() {
	case "RS256":
		return v.verifyRS256(ctx, raw)
	case "HS256":
		if v.cfg.DevHS256Secret == "" {
			return Identity{}, ErrInvalidToken
		}
		return v.verifyHS256(raw)
	default:
		return Identity{}, ErrInvalidToken
	}
}

type authClaims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

func (v *Verifier) verifyRS256(ctx context.Context, raw string) (Identity, error) {
	claims := &authClaims{}
	_, err := v.rs.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("missing kid")
		}
		return v.jwks.key(ctx, kid)
	})
	if err != nil {
		return Identity{}, ErrInvalidToken
	}
	id, err := uuid.Parse(claims.Subject)
	// auth always issues sid; a token without it could never be revoked.
	if err != nil || claims.SessionID == "" {
		return Identity{}, ErrInvalidToken
	}
	return Identity{UserID: id.String(), SessionID: claims.SessionID}, nil
}

type devClaims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

func (v *Verifier) verifyHS256(raw string) (Identity, error) {
	claims := &devClaims{}
	_, err := v.hs.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) {
		return []byte(v.cfg.DevHS256Secret), nil
	})
	if err != nil {
		return Identity{}, ErrInvalidToken
	}
	subject := claims.UserID
	if subject == "" {
		subject = claims.Subject
	}
	id, err := uuid.Parse(subject)
	if err != nil {
		return Identity{}, ErrInvalidToken
	}
	// Only the user is taken from a dev token: role and complex claims are
	// ignored and resolved from the memberships, exactly as for RS256.
	return Identity{UserID: id.String(), DevToken: true}, nil
}

// jwks caches the auth service's public keys. An unknown kid (key rotation)
// triggers a refresh, rate limited so forged kids cannot hammer auth.
type jwks struct {
	url    string
	client *http.Client

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
	lastTry   time.Time
}

func (j *jwks) loaded() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.keys != nil
}

func (j *jwks) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	j.mu.RLock()
	k, ok := j.keys[kid]
	fresh := time.Since(j.fetchedAt) < jwksCacheTTL
	j.mu.RUnlock()
	if ok && fresh {
		return k, nil
	}

	j.mu.Lock()
	if j.keys != nil && time.Since(j.lastTry) < jwksRefreshMinInterval {
		k, ok = j.keys[kid]
		j.mu.Unlock()
		if ok {
			return k, nil
		}
		return nil, errors.New("unknown signing key")
	}
	j.lastTry = time.Now()
	j.mu.Unlock()

	if err := j.refresh(ctx); err != nil {
		if ok { // keep serving a cached key while auth is briefly down
			return k, nil
		}
		return nil, err
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	if k, ok = j.keys[kid]; !ok {
		return nil, errors.New("unknown signing key")
	}
	return k, nil
}

type jwksDoc struct {
	Keys []struct {
		Kty, Kid, Use, N, E string
	} `json:"keys"`
}

func (j *jwks) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.url, nil)
	if err != nil {
		return err
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks: unexpected status %d", resp.StatusCode)
	}
	var doc jwksDoc
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return fmt.Errorf("jwks: %w", err)
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
		n, e := new(big.Int).SetBytes(nb), new(big.Int).SetBytes(eb)
		if n.BitLen() < minRSAKeyBits || !e.IsInt64() || e.Int64() < 3 || e.Int64() > 1<<31-1 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: n, E: int(e.Int64())}
	}
	if len(keys) == 0 {
		return errors.New("jwks: no usable RSA keys")
	}
	j.mu.Lock()
	j.keys, j.fetchedAt = keys, time.Now()
	j.mu.Unlock()
	return nil
}
