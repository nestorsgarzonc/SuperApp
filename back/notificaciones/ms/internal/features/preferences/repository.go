package preferences

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Repository interface {
	GetByUser(ctx context.Context, userID uuid.UUID) ([]*Preference, error)

	GetByUsersCategory(ctx context.Context, userIDs []uuid.UUID, category string) (map[uuid.UUID]*Preference, error)
	Upsert(ctx context.Context, p *Preference) error

	GetTimezones(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]string, error)
	UpsertTimezone(ctx context.Context, userID uuid.UUID, timezone string) error
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const selectColumns = `id, user_id, complex_id, category, enabled, quiet_start, quiet_end, created_at, updated_at`

func scan(row interface{ Scan(...any) error }, p *Preference) error {
	return row.Scan(&p.ID, &p.UserID, &p.ComplexID, &p.Category, &p.Enabled,
		&p.QuietStart, &p.QuietEnd, &p.CreatedAt, &p.UpdatedAt)
}

func (r *PostgresRepository) GetByUser(ctx context.Context, userID uuid.UUID) ([]*Preference, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+selectColumns+` FROM notification_preferences WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query notification preferences: %w", err)
	}
	defer rows.Close()

	prefs := []*Preference{}
	for rows.Next() {
		var p Preference
		if err := scan(rows, &p); err != nil {
			return nil, fmt.Errorf("failed to scan notification preference: %w", err)
		}
		prefs = append(prefs, &p)
	}
	return prefs, rows.Err()
}

func (r *PostgresRepository) GetByUsersCategory(ctx context.Context, userIDs []uuid.UUID, category string) (map[uuid.UUID]*Preference, error) {
	out := make(map[uuid.UUID]*Preference)
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+selectColumns+` FROM notification_preferences
		 WHERE user_id = ANY($1::uuid[]) AND category = $2`,
		pq.Array(uuid.UUIDs(userIDs).Strings()), category)
	if err != nil {
		return nil, fmt.Errorf("failed to query notification preferences: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p Preference
		if err := scan(rows, &p); err != nil {
			return nil, fmt.Errorf("failed to scan notification preference: %w", err)
		}
		out[p.UserID] = &p
	}
	return out, rows.Err()
}

func (r *PostgresRepository) Upsert(ctx context.Context, p *Preference) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO notification_preferences (user_id, complex_id, category, enabled, quiet_start, quiet_end)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, category)
		DO UPDATE SET
			complex_id  = EXCLUDED.complex_id,
			enabled     = EXCLUDED.enabled,
			quiet_start = EXCLUDED.quiet_start,
			quiet_end   = EXCLUDED.quiet_end,
			updated_at  = NOW()`,
		p.UserID, p.ComplexID, p.Category, p.Enabled, p.QuietStart, p.QuietEnd,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert notification preference: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetTimezones(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	out := make(map[uuid.UUID]string)
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id, timezone FROM notification_settings WHERE user_id = ANY($1::uuid[])`,
		pq.Array(uuid.UUIDs(userIDs).Strings()))
	if err != nil {
		return nil, fmt.Errorf("failed to query notification settings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var tz string
		if err := rows.Scan(&id, &tz); err != nil {
			return nil, fmt.Errorf("failed to scan notification settings: %w", err)
		}
		out[id] = tz
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpsertTimezone(ctx context.Context, userID uuid.UUID, timezone string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO notification_settings (user_id, timezone) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET timezone = EXCLUDED.timezone, updated_at = NOW()`,
		userID, timezone)
	if err != nil {
		return fmt.Errorf("failed to upsert timezone: %w", err)
	}
	return nil
}
