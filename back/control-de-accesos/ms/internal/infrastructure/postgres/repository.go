package postgres

import (
	"context"
	"database/sql"
	"errors"

	"superapp/control-de-accesos/internal/domain"
)

type Repository struct{ db *sql.DB }

func New(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Ping(ctx context.Context) error { return r.db.PingContext(ctx) }

func (r *Repository) Actor(ctx context.Context, complexID, userID string) (domain.Actor, error) {
	var fullName string
	err := r.db.QueryRowContext(ctx,
		`SELECT full_name FROM ref_users WHERE id = $1::uuid AND status = 'ACTIVE'`, userID,
	).Scan(&fullName)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Actor{}, domain.ErrUnauthorized
	}
	if err != nil {
		return domain.Actor{}, err
	}
	var guard bool
	if err = r.db.QueryRowContext(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM ref_complex_staff
		   WHERE complex_id = $1::uuid AND user_id = $2::uuid AND role = 'GUARD'
		 )`, complexID, userID,
	).Scan(&guard); err != nil {
		return domain.Actor{}, err
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT unit_id::text FROM ref_unit_members WHERE complex_id = $1::uuid AND user_id = $2::uuid`,
		complexID, userID,
	)
	if err != nil {
		return domain.Actor{}, err
	}
	defer rows.Close()
	units := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return domain.Actor{}, err
		}
		units = append(units, id)
	}
	if err = rows.Err(); err != nil {
		return domain.Actor{}, err
	}
	if !guard && len(units) == 0 {
		return domain.Actor{}, domain.ErrUnauthorized
	}
	role := domain.RoleResident
	if guard {
		role = domain.RoleGuard
	}
	return domain.Actor{ComplexID: complexID, UserID: userID, FullName: fullName, Role: role, UnitIDs: units}, nil
}

func (r *Repository) UnitInComplex(ctx context.Context, complexID, unitID string) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM ref_units WHERE id = $1::uuid AND complex_id = $2::uuid)`,
		unitID, complexID,
	).Scan(&ok)
	return ok, err
}

func (r *Repository) UserInUnit(ctx context.Context, userID, unitID string) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM ref_unit_members WHERE user_id = $1::uuid AND unit_id = $2::uuid)`,
		userID, unitID,
	).Scan(&ok)
	return ok, err
}

const passColumns = `id::text, complex_id::text, unit_id::text, host_user_id::text,
	visitor_name, COALESCE(document, ''), token,
	valid_from, valid_until, status, used_by::text, used_at,
	checked_out_by::text, checked_out_at, created_at, updated_at`

func scanPass(s interface{ Scan(...any) error }) (*domain.Pass, error) {
	var p domain.Pass
	var usedBy, checkedOutBy sql.NullString
	var usedAt, checkedOutAt sql.NullTime
	if err := s.Scan(
		&p.ID, &p.ComplexID, &p.UnitID, &p.HostUserID,
		&p.VisitorName, &p.Document, &p.Token,
		&p.ValidFrom, &p.ValidUntil, &p.Status, &usedBy, &usedAt,
		&checkedOutBy, &checkedOutAt, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if usedBy.Valid {
		p.UsedBy = &usedBy.String
	}
	if usedAt.Valid {
		p.UsedAt = &usedAt.Time
	}
	if checkedOutBy.Valid {
		p.CheckedOutBy = &checkedOutBy.String
	}
	if checkedOutAt.Valid {
		p.CheckedOutAt = &checkedOutAt.Time
	}
	return &p, nil
}

func nullText(s string) any {
	if s == "" {
		return nil
	}
	return s
}
