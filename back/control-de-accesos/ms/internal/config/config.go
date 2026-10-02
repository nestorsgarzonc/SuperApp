package config

import (
	"os"
	"strconv"
)

type Config struct {
	Env        string
	Port       string
	JWTSecret  string
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string
	// DBSSLRootCert, when set, is the path to a CA bundle used for
	// sslmode=verify-full.
	DBSSLRootCert         string
	DBMaxOpenConns        int
	DBMaxIdleConns        int
	DBConnMaxLifetimeMins int
	NotificationsURL      string
}

func Load() Config {
	return Config{
		Env:            getEnv("ENV", "development"),
		Port:           getEnv("PORT", "8080"),
		JWTSecret:      getEnv("JWT_SECRET", ""),
		DBHost:         getEnv("DB_HOST", "localhost"),
		DBPort:         getEnv("DB_PORT", "5432"),
		DBUser:         getEnv("DB_USER", "control_de_accesos"),
		DBPassword:     getEnv("DB_PASSWORD", "control_de_accesos"),
		DBName:         getEnv("DB_NAME", "control_de_accesos"),
		DBSSLMode:      getEnv("DB_SSLMODE", "disable"),
		DBSSLRootCert:  getEnv("DB_SSLROOTCERT", ""),
		DBMaxOpenConns: getEnvInt("DB_MAX_OPEN_CONNS", 10),
		DBMaxIdleConns: getEnvInt("DB_MAX_IDLE_CONNS", 5),
		// Recycling connections periodically means the pool picks up a
		// changed DNS endpoint instead of pinning to a stale backend.
		DBConnMaxLifetimeMins: getEnvInt("DB_CONN_MAX_LIFETIME_MINUTES", 5),
		// Empty disables outbound event notifications (client no-ops).
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
