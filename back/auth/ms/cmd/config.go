package main

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type config struct {
	addr, databaseURL, jwtPrivateKeyFile, jwtIssuer, jwtAuthAudience string
	accessTTL, refreshTTL                                            time.Duration
	gatewayTenantSecret                                              []byte
}

func loadConfig() (config, error) {
	c := config{
		addr:                envOrDefault("AUTH_ADDR", ":8081"),
		databaseURL:         os.Getenv("AUTH_DATABASE_URL"),
		jwtPrivateKeyFile:   os.Getenv("JWT_PRIVATE_KEY_FILE"),
		jwtIssuer:           envOrDefault("JWT_ISSUER", "http://localhost:8081"),
		jwtAuthAudience:     envOrDefault("JWT_AUDIENCE", "superapp-api"),
		accessTTL:           15 * time.Minute,
		refreshTTL:          30 * 24 * time.Hour,
		gatewayTenantSecret: []byte(os.Getenv("GATEWAY_TENANT_HMAC_SECRET")),
	}
	if c.databaseURL == "" {
		return c, errors.New("AUTH_DATABASE_URL is required")
	}
	if c.jwtPrivateKeyFile == "" {
		return c, errors.New("JWT_PRIVATE_KEY_FILE is required")
	}
	if value := os.Getenv("ACCESS_TOKEN_TTL_MINUTES"); value != "" {
		minutes, err := strconv.Atoi(value)
		if err != nil || minutes < 1 || minutes > 60 {
			return c, errors.New("ACCESS_TOKEN_TTL_MINUTES must be 1..60")
		}
		c.accessTTL = time.Duration(minutes) * time.Minute
	}
	if value := os.Getenv("REFRESH_TOKEN_TTL_DAYS"); value != "" {
		days, err := strconv.Atoi(value)
		if err != nil || days < 1 || days > 365 {
			return c, errors.New("REFRESH_TOKEN_TTL_DAYS must be 1..365")
		}
		c.refreshTTL = time.Duration(days) * 24 * time.Hour
	}
	return c, nil
}

func envOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
