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

	// QR system circuit breaker.
	QRBreakerFailures    int
	QRBreakerOpenSeconds int
	QRCallTimeoutMs      int
	QRMaxConcurrent      int
}

func Load() Config {
	return Config{
		// Fail closed: without an explicit ENV=development the service behaves as
		// production (JWT required on every route, no dev tooling).
		Env:            getEnv("ENV", "production"),
		Port:           getEnv("PORT", "8080"),
		JWTSecret:      getEnv("JWT_SECRET", ""),
		DBHost:         getEnv("DB_HOST", "localhost"),
		DBPort:         getEnv("DB_PORT", "5432"),
		DBUser:         getEnv("DB_USER", "control_de_accesos"),
		DBPassword:     getEnv("DB_PASSWORD", ""),
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

		QRBreakerFailures:    getEnvInt("QR_BREAKER_FAILURES", 3),
		QRBreakerOpenSeconds: getEnvInt("QR_BREAKER_OPEN_SECONDS", 10),
		QRCallTimeoutMs:      getEnvInt("QR_CALL_TIMEOUT_MS", 800),
		QRMaxConcurrent:      getEnvInt("QR_MAX_CONCURRENT", 32),
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
