package config

import (
	"errors"
	"os"
)

type Config struct {
	Addr, DatabaseURL string
}

func Load() (Config, error) {
	c := Config{Addr: get("ACCESS_ADDR", ":4000"), DatabaseURL: os.Getenv("ACCESS_DATABASE_URL")}
	if c.DatabaseURL == "" {
		return c, errors.New("ACCESS_DATABASE_URL is required")
	}
	return c, nil
}

func get(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
