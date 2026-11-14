// Package config loads the gateway configuration from the environment and
// refuses to start on combinations that would be unsafe (fail closed).
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Upstream is a microservice behind the gateway.
type Upstream struct {
	URL *url.URL
	// Secret is sent as X-Gateway-Secret together with a verified identity.
	// One secret per service limits the blast radius of a leak.
	Secret string
}

type Config struct {
	Env  string
	Port string

	Auth             Upstream
	ControlDeAccesos Upstream
	Notificaciones   Upstream

	// JWKS of the auth service and the claims its RS256 tokens must carry.
	AuthJWKSURL  string
	AuthIssuer   string
	AuthAudience string
	// AuthTenantSecret signs X-Tenant-ID for the auth service
	// (GATEWAY_TENANT_HMAC_SECRET there). Empty = tenant context not sent.
	AuthTenantSecret string

	// InternalSecret authenticates the gateway against /internal/* of
	// control-de-accesos (membership role lookup).
	InternalSecret string

	RedisURL string

	CORSAllowedOrigins []string

	// Development-only switches. Load rejects them outside ENV=development.
	DevHS256Secret  string // accept HS256 tokens from control-de-accesos /dev/token
	ExposeDevRoutes bool   // route /api/v1/dev/* (control-de-accesos dev tooling)

	// TrustForwardedFor takes the client IP from X-Forwarded-For (rightmost
	// entry). Enable only when every request reaches the gateway through a
	// proxy you control (nginx, ALB); otherwise clients pick their own IP.
	TrustForwardedFor bool

	PublicRatePerMin int
	UserRatePerMin   int
	MaxBodyBytes     int64
	UpstreamTimeout  time.Duration
	RoleCacheTTL     time.Duration
}

func (c Config) IsDevelopment() bool { return c.Env == "development" }

func Load() (Config, error) {
	c := Config{
		// Without an explicit ENV=development the gateway behaves as production.
		Env:                getenv("ENV", "production"),
		Port:               getenv("PORT", "8080"),
		AuthJWKSURL:        os.Getenv("AUTH_JWKS_URL"),
		AuthIssuer:         os.Getenv("AUTH_JWT_ISSUER"),
		AuthAudience:       getenv("AUTH_JWT_AUDIENCE", "superapp-api"),
		AuthTenantSecret:   os.Getenv("AUTH_TENANT_HMAC_SECRET"),
		InternalSecret:     os.Getenv("INTERNAL_SECRET"),
		RedisURL:           os.Getenv("REDIS_URL"),
		CORSAllowedOrigins: splitList(os.Getenv("CORS_ALLOWED_ORIGINS")),
		DevHS256Secret:     os.Getenv("DEV_HS256_SECRET"),
		ExposeDevRoutes:    getbool("EXPOSE_DEV_ROUTES"),
		TrustForwardedFor:  getbool("TRUST_FORWARDED_FOR"),
		PublicRatePerMin:   getint("RATE_LIMIT_PUBLIC_PER_MIN", 10),
		UserRatePerMin:     getint("RATE_LIMIT_USER_PER_MIN", 300),
		MaxBodyBytes:       int64(getint("MAX_BODY_BYTES", 1<<20)),
		UpstreamTimeout:    time.Duration(getint("UPSTREAM_TIMEOUT_SECONDS", 30)) * time.Second,
		RoleCacheTTL:       time.Duration(getint("ROLE_CACHE_TTL_SECONDS", 60)) * time.Second,
	}

	var errs []error
	var err error
	if c.Auth, err = upstream("AUTH_URL", ""); err != nil {
		errs = append(errs, err)
	}
	if c.ControlDeAccesos, err = upstream("CONTROL_ACCESOS_URL", "CONTROL_ACCESOS_SECRET"); err != nil {
		errs = append(errs, err)
	}
	if c.Notificaciones, err = upstream("NOTIFICACIONES_URL", "NOTIFICACIONES_SECRET"); err != nil {
		errs = append(errs, err)
	}
	if c.AuthJWKSURL == "" && c.Auth.URL != nil {
		c.AuthJWKSURL = c.Auth.URL.JoinPath("/.well-known/jwks.json").String()
	}
	if c.AuthIssuer == "" {
		errs = append(errs, errors.New("AUTH_JWT_ISSUER is required (must equal JWT_ISSUER of the auth service)"))
	}
	if c.InternalSecret == "" {
		errs = append(errs, errors.New("INTERNAL_SECRET is required (role lookup in control-de-accesos)"))
	}
	if c.AuthTenantSecret != "" && len(c.AuthTenantSecret) < 32 {
		errs = append(errs, errors.New("AUTH_TENANT_HMAC_SECRET must be at least 32 bytes"))
	}
	if !c.IsDevelopment() {
		if c.DevHS256Secret != "" {
			errs = append(errs, fmt.Errorf("DEV_HS256_SECRET is only allowed with ENV=development (ENV=%s)", c.Env))
		}
		if c.ExposeDevRoutes {
			errs = append(errs, fmt.Errorf("EXPOSE_DEV_ROUTES is only allowed with ENV=development (ENV=%s)", c.Env))
		}
		for _, o := range c.CORSAllowedOrigins {
			if o == "*" {
				errs = append(errs, errors.New("CORS_ALLOWED_ORIGINS=* is only allowed with ENV=development"))
			}
		}
	}
	if c.PublicRatePerMin < 1 || c.UserRatePerMin < 1 || c.MaxBodyBytes < 1 || c.UpstreamTimeout <= 0 || c.RoleCacheTTL <= 0 {
		errs = append(errs, errors.New("rate limits, MAX_BODY_BYTES, UPSTREAM_TIMEOUT_SECONDS and ROLE_CACHE_TTL_SECONDS must be positive"))
	}
	return c, errors.Join(errs...)
}

func upstream(urlKey, secretKey string) (Upstream, error) {
	raw := os.Getenv(urlKey)
	if raw == "" {
		return Upstream{}, fmt.Errorf("%s is required", urlKey)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Upstream{}, fmt.Errorf("%s must be an absolute http(s) URL", urlKey)
	}
	up := Upstream{URL: u}
	if secretKey != "" {
		up.Secret = os.Getenv(secretKey)
		if up.Secret == "" {
			return Upstream{}, fmt.Errorf("%s is required", secretKey)
		}
	}
	return up, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getint(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		return -1 // invalid value: surfaced by the positivity check
	}
	return fallback
}

func getbool(key string) bool {
	b, _ := strconv.ParseBool(os.Getenv(key))
	return b
}

func splitList(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.TrimRight(p, "/"))
		}
	}
	return out
}
