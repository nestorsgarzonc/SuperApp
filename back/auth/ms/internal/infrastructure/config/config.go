package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr, DatabaseURL, JWTPrivateKeyFile, JWTIssuer, JWTAuthAudience string
	AccessTTL, RefreshTTL                                            time.Duration
	GatewayTenantSecret                                              []byte
}

func Load() (Config, error) {
	c := Config{Addr: get("AUTH_ADDR", ":8081"), DatabaseURL: os.Getenv("AUTH_DATABASE_URL"), JWTPrivateKeyFile: os.Getenv("JWT_PRIVATE_KEY_FILE"), JWTIssuer: get("JWT_ISSUER", "http://localhost:8081"), JWTAuthAudience: get("JWT_AUDIENCE", "superapp-api"), AccessTTL: 15 * time.Minute, RefreshTTL: 30 * 24 * time.Hour, GatewayTenantSecret: []byte(os.Getenv("GATEWAY_TENANT_HMAC_SECRET"))}
	if c.DatabaseURL == "" {
		return c, errors.New("AUTH_DATABASE_URL is required")
	}
	if c.JWTPrivateKeyFile == "" {
		return c, errors.New("JWT_PRIVATE_KEY_FILE is required")
	}
	if v := os.Getenv("ACCESS_TOKEN_TTL_MINUTES"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 60 {
			return c, errors.New("ACCESS_TOKEN_TTL_MINUTES must be 1..60")
		}
		c.AccessTTL = time.Duration(n) * time.Minute
	}
	if v := os.Getenv("REFRESH_TOKEN_TTL_DAYS"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 365 {
			return c, errors.New("REFRESH_TOKEN_TTL_DAYS must be 1..365")
		}
		c.RefreshTTL = time.Duration(n) * 24 * time.Hour
	}
	return c, nil
}
func get(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
