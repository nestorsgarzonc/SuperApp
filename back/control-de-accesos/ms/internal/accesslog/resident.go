package accesslog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Residents entering/leaving the building. They use the same access_logs
// table as visitors, with subject_type = RESIDENT (see 006_resident_movements.sql).

var (
	ErrAlreadyInside  = errors.New("you are already inside the building")
	ErrAlreadyOutside = errors.New("you are already outside the building")
	ErrNotResident    = errors.New("you are not a resident of this complex")
	ErrClientIDTaken  = errors.New("client_id already used by another record")
)

const (
	StatusInside  = "INSIDE"
	StatusOutside = "OUTSIDE"
	StatusUnknown = "UNKNOWN" // no movements yet
)

// ResidentMovementRequest: the resident only says which way they're going.
// Name, unit and time are set by the server, never by the client.
type ResidentMovementRequest struct {
	ClientID uuid.UUID `json:"client_id"`
	Movement string    `json:"movement"`
}

func (r ResidentMovementRequest) validate() map[string][]string {
	errs := map[string][]string{}
	if r.ClientID == uuid.Nil {
		errs["client_id"] = []string{"required"}
	}
	if r.Movement != MovementEntry && r.Movement != MovementExit {
		errs["movement"] = []string{"must be ENTRY or EXIT"}
	}
	return errs
}

// checkTransition decides whether `want` is allowed after the resident's last
// movement (empty = no history yet, either direction is allowed).
func checkTransition(last, want string) error {
	switch {
	case last == MovementEntry && want == MovementEntry:
		return ErrAlreadyInside
	case last == MovementExit && want == MovementExit:
		return ErrAlreadyOutside
	}
	return nil
}

// statusFrom derives the current state from the latest movement.
func statusFrom(last string) string {
	switch last {
	case MovementEntry:
		return StatusInside
	case MovementExit:
		return StatusOutside
	}
	return StatusUnknown
}

type ResidentStatus struct {
	Status    string       `json:"status"`
	Since     *time.Time   `json:"since,omitempty"`
	Movements []*AccessLog `json:"movements"`
}

// Presence is one resident currently inside the building.
type Presence struct {
	UserID    uuid.UUID `json:"user_id"`
	Name      string    `json:"name"`
	UnitLabel string    `json:"unit_label"`
	Since     time.Time `json:"since"`
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func loadByClient(ctx context.Context, q queryer, complexID, clientID uuid.UUID) (*AccessLog, error) {
	var l AccessLog
	err := scan(q.QueryRowContext(ctx,
		`SELECT `+selectColumns+` `+fromClause+` WHERE l.complex_id = $1 AND l.client_id = $2`,
		complexID, clientID), &l)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *PostgresRepository) CreateResidentMovement(ctx context.Context, complexID, userID uuid.UUID, req ResidentMovementRequest) (*AccessLog, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Serialize this resident's movements in this complex, so two quick taps
	// can't both pass the "last movement" check.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"resident-movement:"+complexID.String()+":"+userID.String()); err != nil {
		return nil, false, fmt.Errorf("failed to lock resident: %w", err)
	}

	// A retry of the same client_id returns the original record (even if the
	// state has moved on since) instead of a spurious conflict.
	existing, err := loadByClient(ctx, tx, complexID, req.ClientID)
	switch {
	case err == nil:
		if existing.SubjectType != SubjectResident || existing.SubjectUserID != userID {
			return nil, false, ErrClientIDTaken
		}
		return existing, false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return nil, false, fmt.Errorf("failed to check client_id: %w", err)
	}

	var last sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT movement FROM access_logs
		WHERE complex_id = $1 AND subject_type = 'RESIDENT' AND subject_user_id = $2
		ORDER BY occurred_at DESC, recorded_at DESC LIMIT 1`, complexID, userID).Scan(&last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("failed to load last movement: %w", err)
	}
	if err := checkTransition(last.String, req.Movement); err != nil {
		return nil, false, err
	}

	// Identity comes from the directory, not from the request.
	var name, unit string
	err = tx.QueryRowContext(ctx, `
		SELECT u.name, un.name
		FROM users u
		JOIN unit_members um ON um.user_id = u.id
		JOIN units un ON un.id = um.unit_id
		WHERE u.id = $1 AND un.complex_id = $2
		ORDER BY un.name LIMIT 1`, userID, complexID).Scan(&name, &unit)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrNotResident
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to load resident: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO access_logs (
			id, client_id, complex_id, movement, result, visitor_name, unit_label,
			visitor_type, registered_by, occurred_at, subject_type, subject_user_id
		)
		VALUES ($1, $2, $3, $4, 'AUTHORIZED', $5, $6, 'Residente', $7, now(), 'RESIDENT', $7)
		ON CONFLICT (complex_id, client_id) DO NOTHING`,
		uuid.New(), req.ClientID, complexID, req.Movement, name, unit, userID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to insert resident movement: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, false, ErrClientIDTaken
	}

	l, err := loadByClient(ctx, tx, complexID, req.ClientID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to load resident movement: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("failed to commit resident movement: %w", err)
	}
	return l, true, nil
}

func (r *PostgresRepository) ResidentHistory(ctx context.Context, complexID, userID uuid.UUID, limit int) ([]*AccessLog, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+selectColumns+` `+fromClause+`
		 WHERE l.complex_id = $1 AND l.subject_type = 'RESIDENT' AND l.subject_user_id = $2
		 ORDER BY l.occurred_at DESC, l.recorded_at DESC LIMIT $3`, complexID, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list resident movements: %w", err)
	}
	defer rows.Close()

	logs := make([]*AccessLog, 0)
	for rows.Next() {
		var l AccessLog
		if err := scan(rows, &l); err != nil {
			return nil, fmt.Errorf("failed to scan resident movement: %w", err)
		}
		logs = append(logs, &l)
	}
	return logs, rows.Err()
}

func (r *PostgresRepository) Presence(ctx context.Context, complexID uuid.UUID) ([]*Presence, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT subject_user_id, visitor_name, unit_label, occurred_at FROM (
			SELECT DISTINCT ON (subject_user_id) subject_user_id, visitor_name, unit_label, movement, occurred_at
			FROM access_logs
			WHERE complex_id = $1 AND subject_type = 'RESIDENT'
			ORDER BY subject_user_id, occurred_at DESC, recorded_at DESC
		) latest
		WHERE movement = 'ENTRY'
		ORDER BY occurred_at DESC`, complexID)
	if err != nil {
		return nil, fmt.Errorf("failed to load presence: %w", err)
	}
	defer rows.Close()

	out := make([]*Presence, 0)
	for rows.Next() {
		var p Presence
		if err := rows.Scan(&p.UserID, &p.Name, &p.UnitLabel, &p.Since); err != nil {
			return nil, fmt.Errorf("failed to scan presence: %w", err)
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}
