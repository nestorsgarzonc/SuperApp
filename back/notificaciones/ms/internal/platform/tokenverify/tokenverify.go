// Package tokenverify valida los Bearer tokens que llegan SIN pasar por el
// api-gateway (p. ej. detrás de AWS API Gateway, que no inyecta identidad):
//
//   - RS256 emitidos por auth: firma con su JWKS, iss, aud y exp.
//   - HS256 del login de pruebas de control-de-accesos (/dev/token), firmados
//     con el mismo JWT_SECRET.
//
// Solo se acepta el algoritmo para el que hay llave configurada, así que
// "none" y la confusión RS256→HS256 quedan cerrados.
package tokenverify

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

type Config struct {
	JWKSURL    string // vacío = RS256 desactivado
	Issuer     string
	Audience   string
	HS256Key   string // vacío = HS256 desactivado
	HTTPClient *http.Client
}

type Verifier struct {
	cfg  Config
	jwks *jwks
	rs   *jwt.Parser
	hs   *jwt.Parser
}

func New(cfg Config) (*Verifier, error) {
	if cfg.JWKSURL == "" && cfg.HS256Key == "" {
		return nil, errors.New("tokenverify: AUTH_JWKS_URL or JWT_SECRET is required")
	}
	if cfg.JWKSURL != "" && (cfg.Issuer == "" || cfg.Audience == "") {
		return nil, errors.New("tokenverify: AUTH_JWT_ISSUER and AUTH_JWT_AUDIENCE are required with AUTH_JWKS_URL")
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 3 * time.Second}
	}
	return &Verifier{
		cfg:  cfg,
		jwks: &jwks{url: cfg.JWKSURL, client: cfg.HTTPClient},
		rs: jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(cfg.Issuer),
			jwt.WithAudience(cfg.Audience), jwt.WithExpirationRequired(), jwt.WithLeeway(leeway)),
		hs: jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithLeeway(leeway)),
	}, nil
}

type claims struct {
	UserID string `json:"user_id"` // tokens HS256 de pruebas
	jwt.RegisteredClaims
}

// Subject devuelve el usuario de un token válido.
func (v *Verifier) Subject(ctx context.Context, raw string) (uuid.UUID, error) {
	unverified, _, err := jwt.NewParser().ParseUnverified(raw, &claims{})
	if err != nil {
		return uuid.Nil, ErrInvalidToken
	}
	c := &claims{}
	switch alg := unverified.Method.Alg(); {
	case alg == "RS256" && v.cfg.JWKSURL != "":
		_, err = v.rs.ParseWithClaims(raw, c, func(t *jwt.Token) (any, error) {
			kid, _ := t.Header["kid"].(string)
			if kid == "" {
				return nil, errors.New("missing kid")
			}
			return v.jwks.key(ctx, kid)
		})
	case alg == "HS256" && v.cfg.HS256Key != "":
		_, err = v.hs.ParseWithClaims(raw, c, func(*jwt.Token) (any, error) { return []byte(v.cfg.HS256Key), nil })
	default:
		return uuid.Nil, ErrInvalidToken
	}
	if err != nil {
		return uuid.Nil, ErrInvalidToken
	}
	subject := c.Subject
	if subject == "" {
		subject = c.UserID
	}
	id, err := uuid.Parse(subject)
	if err != nil {
		return uuid.Nil, ErrInvalidToken
	}
	return id, nil
}

// jwks guarda en caché las llaves públicas de auth. Un kid desconocido
// (rotación) fuerza un refresco, limitado para que un kid falso no sature auth.
type jwks struct {
	url    string
	client *http.Client

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
	lastTry   time.Time
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
		if ok {
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
	var doc struct {
		Keys []struct{ Kty, Kid, Use, N, E string } `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return fmt.Errorf("jwks: %w", err)
	}
	keys := map[string]*rsa.PublicKey{}
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
