package notifications

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

var (
	ErrNotFound       = errors.New("notification not found")
	ErrDuplicateEvent = errors.New("event already processed")
)

type Repository interface {
	CreateNotifications(ctx context.Context, in NewNotifications) ([]*Notification, error)

	GetUserNotifications(ctx context.Context, complexID, userID uuid.UUID, unreadOnly bool, types []string, limit int) ([]*Notification, error)
	CountUnread(ctx context.Context, complexID, userID uuid.UUID) (int, error)
	MarkAsRead(ctx context.Context, id, complexID, userID uuid.UUID) error
	MarkAllAsRead(ctx context.Context, complexID, userID uuid.UUID) (int64, error)
	UpdateDelivery(ctx context.Context, id uuid.UUID, status string, deliveredAt *time.Time) error
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateNotifications(ctx context.Context, in NewNotifications) ([]*Notification, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO processed_events (event_id, source) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		in.EventID, in.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to record processed event: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrDuplicateEvent
	}

	created := make([]*Notification, 0, len(in.UserIDs))
	if len(in.UserIDs) > 0 {

		rows, err := tx.QueryContext(ctx, `
			INSERT INTO notifications (complex_id, user_id, title, body, data, type, source_event_id)
			SELECT $1::uuid, u, $3::text, $4::text, $5::jsonb, $6::text, $7::text
			FROM unnest($2::uuid[]) AS u
			RETURNING id, user_id, created_at, delivery_status`,
			in.ComplexID, pq.Array(uuid.UUIDs(in.UserIDs).Strings()), in.Title, in.Body, in.Data,
			in.Type, in.EventID)
		if err != nil {
			return nil, fmt.Errorf("failed to insert notifications: %w", err)
		}
		for rows.Next() {
			n := &Notification{ComplexID: in.ComplexID, Title: in.Title, Body: in.Body, Data: in.Data, Type: in.Type}
			if err := rows.Scan(&n.ID, &n.UserID, &n.CreatedAt, &n.DeliveryStatus); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan inserted notification: %w", err)
			}
			created = append(created, n)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("failed to insert notifications: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit notifications: %w", err)
	}
	return created, nil
}

func (r *PostgresRepository) GetUserNotifications(ctx context.Context, complexID, userID uuid.UUID, unreadOnly bool, types []string, limit int) ([]*Notification, error) {
	query := `
		SELECT id, complex_id, user_id, title, body, data, type, is_read, read_at,
		       delivery_status, delivered_at, created_at
		FROM notifications
		WHERE (complex_id = $1 OR complex_id IS NULL) AND user_id = $2`
	args := []any{complexID, userID, limit}
	if unreadOnly {
		query += ` AND is_read = false`
	}
	if types != nil {
		args = append(args, pq.Array(types))
		query += ` AND type = ANY($4)`
	}
	query += ` ORDER BY created_at DESC LIMIT $3`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query notifications: %w", err)
	}
	defer rows.Close()

	out := []*Notification{}
	for rows.Next() {
		var n Notification
		if err := rows.Scan(
			&n.ID, &n.ComplexID, &n.UserID, &n.Title, &n.Body, &n.Data, &n.Type,
			&n.IsRead, &n.ReadAt, &n.DeliveryStatus, &n.DeliveredAt, &n.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan notification: %w", err)
		}
		out = append(out, &n)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CountUnread(ctx context.Context, complexID, userID uuid.UUID) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications
		 WHERE (complex_id = $1 OR complex_id IS NULL) AND user_id = $2 AND is_read = false`,
		complexID, userID,
	).Scan(&count)
	return count, err
}

func (r *PostgresRepository) MarkAsRead(ctx context.Context, id, complexID, userID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE notifications SET is_read = true, read_at = COALESCE(read_at, NOW())
		WHERE id = $1 AND (complex_id = $2 OR complex_id IS NULL) AND user_id = $3`, id, complexID, userID)
	if err != nil {
		return fmt.Errorf("failed to mark notification as read: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) MarkAllAsRead(ctx context.Context, complexID, userID uuid.UUID) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE notifications SET is_read = true, read_at = NOW()
		WHERE (complex_id = $1 OR complex_id IS NULL) AND user_id = $2 AND is_read = false`, complexID, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to mark all as read: %w", err)
	}
	return res.RowsAffected()
}

func (r *PostgresRepository) UpdateDelivery(ctx context.Context, id uuid.UUID, status string, deliveredAt *time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET delivery_status = $2, delivered_at = $3 WHERE id = $1`,
		id, status, deliveredAt)
	if err != nil {
		return fmt.Errorf("failed to update notification delivery: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
