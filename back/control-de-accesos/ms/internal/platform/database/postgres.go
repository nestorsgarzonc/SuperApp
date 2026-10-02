package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

type PostgresConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	// SSLRootCert, when set, points at a CA bundle (e.g. RDS's
	// global-bundle.pem) so sslmode=verify-full can validate the server cert.
	SSLRootCert     string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type PostgresClient struct {
	DB *sql.DB
}

func NewPostgresClient(cfg PostgresConfig) (*PostgresClient, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.SSLMode,
	)
	if cfg.SSLRootCert != "" {
		dsn += fmt.Sprintf(" sslrootcert=%s", cfg.SSLRootCert)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PostgresClient{DB: db}, nil
}

func (c *PostgresClient) Close() error {
	return c.DB.Close()
}
