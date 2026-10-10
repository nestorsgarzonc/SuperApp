package config

import (
	"os"
	"strconv"
)

type Config struct {
	Env       string
	Port      string
	JWTSecret string
	// GatewaySecret authenticates requests forwarded by the api-gateway
	// (X-Gateway-Secret); InternalSecret authenticates /internal/* calls
	// (X-Internal-Secret). Empty GatewaySecret = gateway identity not accepted;
	// empty InternalSecret = /internal/* not mounted.
	GatewaySecret  string
	InternalSecret string
	DBHost         string
	DBPort         string
	DBUser         string
	DBPassword     string
	DBName         string
	DBSSLMode      string
	// DBSSLRootCert, when set, is the path to a CA bundle used for
	// sslmode=verify-full.
	DBSSLRootCert         string
	DBMaxOpenConns        int
	DBMaxIdleConns        int
	DBConnMaxLifetimeMins int
	// AuthJWKSURL enables RS256 tokens issued by the auth service (public keys
	// fetched from this URL); issuer and audience must match its tokens.
	AuthJWKSURL  string
	AuthIssuer   string
	AuthAudience string
	// NotificacionesURL is the base URL of the notificaciones service, which
	// receives notification requests on POST /internal/notifications
	// (authenticated with InternalSecret).
	NotificacionesURL string

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
		GatewaySecret:  getEnv("GATEWAY_SECRET", ""),
		InternalSecret: getEnv("INTERNAL_SECRET", ""),
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
		AuthJWKSURL:           getEnv("AUTH_JWKS_URL", ""),
		AuthIssuer:            getEnv("AUTH_JWT_ISSUER", ""),
		AuthAudience:          getEnv("AUTH_JWT_AUDIENCE", "superapp-api"),
		// Empty NOTIFICACIONES_URL disables outbound notifications (client no-ops).
		NotificacionesURL: getEnv("NOTIFICACIONES_URL", ""),

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
