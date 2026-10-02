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
	"superapp/auth/internal/domain"
)

func main() {
	dsn := os.Getenv("AUTH_DATABASE_URL")
	email := domain.NormalizeEmail(os.Getenv("AUTH_BOOTSTRAP_EMAIL"))
	password := os.Getenv("AUTH_BOOTSTRAP_PASSWORD")
	_ = os.Unsetenv("AUTH_BOOTSTRAP_PASSWORD")
	if dsn == "" || email == "" || password == "" {
		fatal("AUTH_DATABASE_URL, AUTH_BOOTSTRAP_EMAIL and AUTH_BOOTSTRAP_PASSWORD are required")
	}
	if len(password) < 12 || len(password) > 72 {
		fatal("bootstrap password must be 12..72 bytes")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fatal(err.Error())
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		fatal(err.Error())
	}
	if _, err = db.ExecContext(ctx, `SELECT pg_advisory_lock(721904118)`); err != nil {
		fatal(err.Error())
	}
	defer db.ExecContext(context.Background(), `SELECT pg_advisory_unlock(721904118)`)
	var count int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM auth_users`).Scan(&count); err != nil {
		fatal(err.Error())
	}
	if count != 0 {
		fatal("bootstrap is allowed only before the first Auth user exists")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		fatal(err.Error())
	}
	uid := uuid.NewString()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		fatal(err.Error())
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO auth_users(id,email,email_normalized,full_name,password_hash,status) VALUES($1,$2,$2,'Platform administrator',$3,'ACTIVE')`, uid, email, string(hash))
	if err != nil {
		fatal(err.Error())
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO auth_user_roles(id,user_id,role_id,assigned_by) SELECT $1,$2,id,$2 FROM auth_roles WHERE code='AUTH_PLATFORM_ADMIN' AND scope='SYSTEM' AND tenant_id IS NULL`, uuid.NewString(), uid)
	if err != nil {
		fatal(err.Error())
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		fatal(errors.New("bootstrap role missing; run cmd/setup first").Error())
	}
	if err = tx.Commit(); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("Bootstrapped Auth administrator %s\n", strings.TrimSpace(email))
}
func fatal(v string) { fmt.Fprintln(os.Stderr, v); os.Exit(1) }
