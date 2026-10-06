package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
	"superapp/auth/internal/core/domain"
)

func main() {
	if err := run(); err != nil {
		fatal(err.Error())
	}
}

func run() error {
	dsn := os.Getenv("AUTH_DATABASE_URL")
	email := domain.NormalizeEmail(os.Getenv("AUTH_BOOTSTRAP_EMAIL"))
	password := os.Getenv("AUTH_BOOTSTRAP_PASSWORD")
	_ = os.Unsetenv("AUTH_BOOTSTRAP_PASSWORD")
	if dsn == "" || email == "" || password == "" {
		return errors.New("AUTH_DATABASE_URL, AUTH_BOOTSTRAP_EMAIL and AUTH_BOOTSTRAP_PASSWORD are required")
	}
	if len(password) < 12 || len(password) > 72 {
		return errors.New("bootstrap password must be 12..72 bytes")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(721904118)`); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM auth_users`).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return errors.New("bootstrap is allowed only before the first Auth user exists")
	}
	uid := uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO auth_users(id,email,email_normalized,full_name,password_hash,status) VALUES($1,$2,$2,'Platform administrator',$3,'ACTIVE')`, uid, email, string(hash))
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO auth_user_roles(id,user_id,role_id,assigned_by) SELECT $1,$2,id,$2 FROM auth_roles WHERE code='AUTH_PLATFORM_ADMIN' AND scope='SYSTEM' AND tenant_id IS NULL`, uuid.NewString(), uid)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("bootstrap role missing; run cmd/setup first")
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("Bootstrapped Auth administrator %s\n", strings.TrimSpace(email))
	return nil
}
func fatal(v string) { fmt.Fprintln(os.Stderr, v); os.Exit(1) }
