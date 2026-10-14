package preferences

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Repository interface {
	GetByUser(ctx context.Context, userID uuid.UUID) (map[string]bool, error)
	MutedUsers(ctx context.Context, userIDs []uuid.UUID, group string) (map[uuid.UUID]bool, error)
	Save(ctx context.Context, userID uuid.UUID, enabled map[string]bool) error
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) GetByUser(ctx context.Context, userID uuid.UUID) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT group_name, enabled FROM notification_preferences WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query notification preferences: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var group string
		var enabled bool
		if err := rows.Scan(&group, &enabled); err != nil {
			return nil, fmt.Errorf("failed to scan notification preference: %w", err)
		}
		out[group] = enabled
	}
	return out, rows.Err()
}

func (r *PostgresRepository) MutedUsers(ctx context.Context, userIDs []uuid.UUID, group string) (map[uuid.UUID]bool, error) {
	out := map[uuid.UUID]bool{}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT user_id FROM notification_preferences
		WHERE user_id = ANY($1::uuid[]) AND group_name = $2 AND NOT enabled`,
		pq.Array(uuid.UUIDs(userIDs).Strings()), group)
	if err != nil {
		return nil, fmt.Errorf("failed to query muted users: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan muted user: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (r *PostgresRepository) Save(ctx context.Context, userID uuid.UUID, enabled map[string]bool) error {
	groups := make([]string, 0, len(enabled))
	values := make([]bool, 0, len(enabled))
	for g, e := range enabled {
		groups = append(groups, g)
		values = append(values, e)
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO notification_preferences (user_id, group_name, enabled)
		SELECT $1, g, e FROM unnest($2::text[], $3::boolean[]) AS t(g, e)
		ON CONFLICT (user_id, group_name)
		DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = NOW()`,
		userID, pq.Array(groups), pq.Array(values))
	if err != nil {
		return fmt.Errorf("failed to save notification preferences: %w", err)
	}
	return nil
}
