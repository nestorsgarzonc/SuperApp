package config

import "os"

// Config holds the service's runtime configuration, loaded from environment
// variables so the same binary runs unchanged in Docker, CI, or bare metal.
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
	NotificationsURL string
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
		DBSSLMode:  getEnv("DB_SSLMODE", "disable"),
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
