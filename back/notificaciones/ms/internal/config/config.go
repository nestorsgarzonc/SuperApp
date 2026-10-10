package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Env                 string
	Port                string
	DatabaseURL         string
	DBMaxOpenConns      int
	DBMaxIdleConns      int
	GatewaySecret       string
	InternalSecret      string
	FirebaseCredentials string
	FirebaseProjectID   string
}

func Load() (Config, error) {
	c := Config{
		Env:                 getenv("ENV", "development"),
		Port:                getenv("PORT", "8083"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		DBMaxOpenConns:      getint("DB_MAX_OPEN_CONNS", 10),
		DBMaxIdleConns:      getint("DB_MAX_IDLE_CONNS", 5),
		GatewaySecret:       os.Getenv("GATEWAY_SECRET"),
		InternalSecret:      os.Getenv("INTERNAL_SECRET"),
		FirebaseCredentials: os.Getenv("FIREBASE_CREDENTIALS_FILE"),
		FirebaseProjectID:   os.Getenv("FIREBASE_PROJECT_ID"),
	}

	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}

	if !c.IsDevelopment() && c.GatewaySecret == "" {
		return c, fmt.Errorf("GATEWAY_SECRET is required when ENV=%s", c.Env)
	}
	if !c.IsDevelopment() && c.InternalSecret == "" {
		return c, fmt.Errorf("INTERNAL_SECRET is required when ENV=%s", c.Env)
	}
	return c, nil
}

func (c Config) IsDevelopment() bool { return c.Env == "development" }

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getint(key string, fallback int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return fallback
}
