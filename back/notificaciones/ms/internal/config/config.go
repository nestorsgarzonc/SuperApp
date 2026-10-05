package config

import (
	"fmt"
	"os"
)

type Config struct {
	Env                 string
	Port                string
	DatabaseURL         string
	RedisURL            string
	EventsStream        string
	EventsGroup         string
	ConsumerName        string
	GatewaySecret       string
	FirebaseCredentials string
	FirebaseProjectID   string
}

func Load() (Config, error) {
	hostname, _ := os.Hostname()
	c := Config{
		Env:                 getenv("ENV", "development"),
		Port:                getenv("PORT", "8083"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		RedisURL:            getenv("REDIS_URL", "redis://localhost:6379/0"),
		EventsStream:        getenv("EVENTS_STREAM", "notifications.requested"),
		EventsGroup:         getenv("EVENTS_GROUP", "notificaciones"),
		ConsumerName:        getenv("CONSUMER_NAME", hostname),
		GatewaySecret:       os.Getenv("GATEWAY_SECRET"),
		FirebaseCredentials: os.Getenv("FIREBASE_CREDENTIALS_FILE"),
		FirebaseProjectID:   os.Getenv("FIREBASE_PROJECT_ID"),
	}

	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}

	if !c.IsDevelopment() && c.GatewaySecret == "" {
		return c, fmt.Errorf("GATEWAY_SECRET is required when ENV=%s", c.Env)
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
