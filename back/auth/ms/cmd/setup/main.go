package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const advisoryLockID int64 = 721904118

func main() {
	dsn := os.Getenv("AUTH_DATABASE_URL")
	if dsn == "" {
		fatal("AUTH_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pgConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		fatal(err.Error())
	}
	pgConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	conn, err := pgx.ConnectConfig(ctx, pgConfig)
	if err != nil {
		fatal(err.Error())
	}
	defer conn.Close(context.Background())
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockID); err != nil {
		fatal(err.Error())
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, advisoryLockID)
	base := os.Getenv("AUTH_DB_DIR")
	if base == "" {
		base = filepath.Join("..", "db")
	}
	if err = apply(ctx, conn, filepath.Join(base, "migrations"), "auth_schema_migrations", "_up.sql"); err != nil {
		fatal(err.Error())
	}
	if err = apply(ctx, conn, filepath.Join(base, "seeds"), "auth_seed_runs", ".sql"); err != nil {
		fatal(err.Error())
	}
	fmt.Println("Auth database schema and seeds are current")
}

func apply(ctx context.Context, conn *pgx.Conn, dir, trackingTable, suffix string) error {
	// trackingTable is a compile-time constant at each call site.
	if _, err := conn.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s(name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`, trackingTable)); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	names := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var found bool
		if err = conn.QueryRow(ctx, fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE name=$1)`, trackingTable), name).Scan(&found); err != nil {
			return err
		}
		if found {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(body)); err == nil {
			_, err = tx.Exec(ctx, fmt.Sprintf(`INSERT INTO %s(name) VALUES($1)`, trackingTable), name)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		fmt.Printf("applied %s\n", name)
	}
	return nil
}

func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
