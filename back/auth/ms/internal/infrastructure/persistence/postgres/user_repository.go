package postgres

import (
	"context"
	"database/sql"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"superapp/auth/internal/domain"
)

func (r *Repository) CreateUser(ctx context.Context, id, email, hash, name string) (*domain.User, error) {
	u := &domain.User{}
	err := r.db.QueryRowContext(ctx, `INSERT INTO auth_users(id,email,email_normalized,password_hash,full_name) VALUES($1,$2,$2,$3,$4) ON CONFLICT(email_normalized) DO NOTHING RETURNING id,email,full_name,status,created_at`, id, email, hash, name).Scan(&u.ID, &u.Email, &u.FullName, &u.Status, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrEmailTaken
	}
	return u, err
}

func (r *Repository) UserByEmail(ctx context.Context, email string) (*domain.User, string, error) {
	u := &domain.User{}
	var hash string
	err := r.db.QueryRowContext(ctx, `SELECT id,email,full_name,status,created_at,password_hash FROM auth_users WHERE email_normalized=$1`, email).Scan(&u.ID, &u.Email, &u.FullName, &u.Status, &u.CreatedAt, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	return u, hash, err
}

func (r *Repository) UserByID(ctx context.Context, id string) (*domain.User, error) {
	u := &domain.User{}
	err := r.db.QueryRowContext(ctx, `SELECT id,email,full_name,status,created_at FROM auth_users WHERE id=$1`, id).Scan(&u.ID, &u.Email, &u.FullName, &u.Status, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return u, err
}

func (r *Repository) UpdateUser(ctx context.Context, id, email, name string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE auth_users SET email=$2,email_normalized=$2,full_name=$3,updated_at=now() WHERE id=$1`, id, email, name)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrEmailTaken
		}
		return translateError(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Repository) SetLastLogin(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE auth_users SET last_login_at=now() WHERE id=$1`, id)
	return err
}

func (r *Repository) SetUserStatus(ctx context.Context, user, status string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE auth_users SET status=$2,updated_at=now() WHERE id=$1`, user, status)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	if status != "ACTIVE" {
		_, err = tx.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`, user)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE auth_refresh_tokens t SET revoked_at=COALESCE(t.revoked_at,now()) FROM auth_sessions s WHERE t.session_id=s.id AND s.user_id=$1`, user)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
