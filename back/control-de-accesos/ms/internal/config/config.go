package config

import (
	"os"
	"strconv"
)

// Config holds the service's runtime configuration, loaded from environment
// variables so the same binary runs unchanged against the local Docker
// Postgres or, later, an AWS RDS instance — only env vars change, not code.
type Config struct {
	Env              string
	Port             string
	JWTSecret        string
	DBHost           string
	DBPort           string
	DBUser           string
	DBPassword       string
	DBName           string
	DBSSLMode        string
	// DBSSLRootCert, when set, is the path to a CA bundle (e.g. RDS's
	// global-bundle.pem) used for sslmode=verify-full against RDS.
	DBSSLRootCert         string
	DBMaxOpenConns        int
	DBMaxIdleConns        int
	DBConnMaxLifetimeMins int
	NotificationsURL      string
}

func Load() Config {
	return Config{
		Env:        getEnv("ENV", "development"),
		Port:       getEnv("PORT", "8080"),
		JWTSecret:  getEnv("JWT_SECRET", ""),
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "control_de_accesos"),
		DBPassword: getEnv("DB_PASSWORD", "control_de_accesos"),
		DBName:     getEnv("DB_NAME", "control_de_accesos"),
		// "disable" matches the local Docker Postgres (no SSL configured).
		// Against RDS set DB_SSLMODE=require (or verify-full + DB_SSLROOTCERT).
		DBSSLMode:     getEnv("DB_SSLMODE", "disable"),
		DBSSLRootCert: getEnv("DB_SSLROOTCERT", ""),
		// Pool defaults are conservative so a single instance never starves a
		// small RDS class's max_connections; override via env per environment.
		DBMaxOpenConns: getEnvInt("DB_MAX_OPEN_CONNS", 10),
		DBMaxIdleConns: getEnvInt("DB_MAX_IDLE_CONNS", 5),
		// RDS can fail over or rotate endpoints under the same DNS name;
		// recycling connections periodically ensures the pool picks that up
		// instead of pinning to a stale backend.
		DBConnMaxLifetimeMins: getEnvInt("DB_CONN_MAX_LIFETIME_MINUTES", 5),
		// Empty by default: the notificaciones microservice doesn't exist yet.
		// The notifier client no-ops until this is set.
		NotificationsURL: getEnv("NOTIFICATIONS_URL", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
