package devices

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

var ErrNotFound = errors.New("device not found")

type Repository interface {
	Upsert(ctx context.Context, d *Device) (*Device, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*Device, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error

	DeleteByToken(ctx context.Context, token string, userID uuid.UUID) error

	TokensByUserIDs(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID][]string, error)

	DeleteTokens(ctx context.Context, tokens []string) error
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Upsert(ctx context.Context, d *Device) (*Device, error) {
	out := *d
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO user_devices (user_id, platform, push_token, device_name, app_version)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (push_token) DO UPDATE SET
			user_id      = EXCLUDED.user_id,
			platform     = EXCLUDED.platform,
			device_name  = EXCLUDED.device_name,
			app_version  = EXCLUDED.app_version,
			last_seen_at = NOW()
		RETURNING id, last_seen_at, created_at`,
		d.UserID, d.Platform, d.PushToken, d.DeviceName, d.AppVersion,
	).Scan(&out.ID, &out.LastSeenAt, &out.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert device: %w", err)
	}
	return &out, nil
}

func (r *PostgresRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]*Device, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, platform, push_token, device_name, app_version, last_seen_at, created_at
		FROM user_devices WHERE user_id = $1 ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query devices: %w", err)
	}
	defer rows.Close()

	out := []*Device{}
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.UserID, &d.Platform, &d.PushToken, &d.DeviceName,
			&d.AppVersion, &d.LastSeenAt, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan device: %w", err)
		}
		out = append(out, &d)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM user_devices WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("failed to delete device: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) DeleteByToken(ctx context.Context, token string, userID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM user_devices WHERE push_token = $1 AND user_id = $2`, token, userID)
	if err != nil {
		return fmt.Errorf("failed to delete device: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) TokensByUserIDs(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	out := make(map[uuid.UUID][]string, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id, push_token FROM user_devices WHERE user_id = ANY($1::uuid[])`, pq.Array(uuid.UUIDs(userIDs).Strings()))
	if err != nil {
		return nil, fmt.Errorf("failed to query device tokens: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var userID uuid.UUID
		var token string
		if err := rows.Scan(&userID, &token); err != nil {
			return nil, fmt.Errorf("failed to scan device token: %w", err)
		}
		out[userID] = append(out[userID], token)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) DeleteTokens(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM user_devices WHERE push_token = ANY($1)`, pq.Array(tokens)); err != nil {
		return fmt.Errorf("failed to delete stale tokens: %w", err)
	}
	return nil
}
