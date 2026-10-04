package accesslog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	// Create inserts a movement. created is false when (complex_id, client_id)
	// already existed; the stored row is returned either way.
	Create(ctx context.Context, complexID, registeredBy uuid.UUID, req CreateRequest, occurredAt time.Time) (log *AccessLog, created bool, err error)
	List(ctx context.Context, complexID uuid.UUID, f ListFilter) ([]*AccessLog, error)

	// Residents entering/leaving the building (see resident.go).
	CreateResidentMovement(ctx context.Context, complexID, userID uuid.UUID, req ResidentMovementRequest) (log *AccessLog, created bool, err error)
	ResidentHistory(ctx context.Context, complexID, userID uuid.UUID, limit int) ([]*AccessLog, error)
	Presence(ctx context.Context, complexID uuid.UUID) ([]*Presence, error)
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

// users is this service's local read-model; the name is best-effort.
const selectColumns = `l.id, l.client_id, l.complex_id, l.movement, l.result, l.subject_type,
	COALESCE(l.subject_user_id, '00000000-0000-0000-0000-000000000000'::uuid), l.visitor_name, l.unit_label,
	l.visitor_type, l.vehicle, l.grant_id, l.registered_by, COALESCE(u.name, ''), l.occurred_at, l.recorded_at`

const fromClause = `FROM access_logs l LEFT JOIN users u ON u.id = l.registered_by`

func scan(row interface{ Scan(dest ...any) error }, l *AccessLog) error {
	return row.Scan(&l.ID, &l.ClientID, &l.ComplexID, &l.Movement, &l.Result, &l.SubjectType, &l.SubjectUserID, &l.VisitorName, &l.UnitLabel,
		&l.VisitorType, &l.Vehicle, &l.GrantID, &l.RegisteredBy, &l.RegisteredByName, &l.OccurredAt, &l.RecordedAt)
}

func (r *PostgresRepository) Create(ctx context.Context, complexID, registeredBy uuid.UUID, req CreateRequest, occurredAt time.Time) (*AccessLog, bool, error) {
	// grant_id is only kept when the grant belongs to the caller's complex,
	// so a log can never reference another tenant's pass.
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO access_logs (
			id, client_id, complex_id, movement, result, visitor_name, unit_label,
			visitor_type, vehicle, grant_id, registered_by, occurred_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9,
			(SELECT id FROM access_grants WHERE id = $10 AND complex_id = $3),
			$11, $12)
		ON CONFLICT (complex_id, client_id) DO NOTHING
	`, uuid.New(), req.ClientID, complexID, req.Movement, req.Result, req.VisitorName, req.UnitLabel,
		req.VisitorType, req.Vehicle, req.GrantID, registeredBy, occurredAt)
	if err != nil {
		return nil, false, fmt.Errorf("failed to insert access log: %w", err)
	}
	n, _ := res.RowsAffected()

	var l AccessLog
	err = scan(r.db.QueryRowContext(ctx,
		`SELECT `+selectColumns+` `+fromClause+` WHERE l.complex_id = $1 AND l.client_id = $2`,
		complexID, req.ClientID), &l)
	if err != nil {
		return nil, false, fmt.Errorf("failed to load access log: %w", err)
	}
	return &l, n == 1, nil
}

func (r *PostgresRepository) List(ctx context.Context, complexID uuid.UUID, f ListFilter) ([]*AccessLog, error) {
	query := `SELECT ` + selectColumns + ` ` + fromClause + ` WHERE l.complex_id = $1`
	args := []any{complexID}
	add := func(cond string, v any) {
		args = append(args, v)
		query += fmt.Sprintf(" AND "+cond, len(args))
	}
	if f.From != nil {
		add("l.occurred_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("l.occurred_at < $%d", *f.To)
	}
	if f.Movement != "" {
		add("l.movement = $%d", f.Movement)
	}
	if f.Result != "" {
		add("l.result = $%d", f.Result)
	}
	if f.Subject != "" {
		add("l.subject_type = $%d", f.Subject)
	}
	args = append(args, f.Limit)
	query += fmt.Sprintf(" ORDER BY l.occurred_at DESC, l.id LIMIT $%d", len(args))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list access logs: %w", err)
	}
	defer rows.Close()

	logs := make([]*AccessLog, 0)
	for rows.Next() {
		var l AccessLog
		if err := scan(rows, &l); err != nil {
			return nil, fmt.Errorf("failed to scan access log: %w", err)
		}
		logs = append(logs, &l)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return logs, nil
}
