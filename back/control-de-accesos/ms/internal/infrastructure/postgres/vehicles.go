package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"superapp/control-de-accesos/internal/domain"
)

func (r *Repository) CreatePreauth(ctx context.Context, p *domain.Preauth) error {
	return r.db.QueryRowContext(ctx,
		`INSERT INTO visitor_vehicle_preauths
		   (complex_id, unit_id, host_user_id, plate, visitor_name, valid_from, valid_until)
		 VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7)
		 RETURNING id::text, status, created_at`,
		p.ComplexID, p.UnitID, p.HostUserID, p.Plate, p.VisitorName, p.ValidFrom, p.ValidUntil,
	).Scan(&p.ID, &p.Status, &p.CreatedAt)
}

func (r *Repository) ListPreauths(ctx context.Context, complexID, hostID string) ([]domain.Preauth, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id::text, complex_id::text, unit_id::text, host_user_id::text, plate, visitor_name,
		        valid_from, valid_until, status, created_at
		 FROM visitor_vehicle_preauths
		 WHERE complex_id = $1::uuid AND host_user_id = $2::uuid AND status = 'active'
		 ORDER BY valid_until DESC
		 LIMIT 200`,
		complexID, hostID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Preauth{}
	for rows.Next() {
		var p domain.Preauth
		if err = rows.Scan(&p.ID, &p.ComplexID, &p.UnitID, &p.HostUserID, &p.Plate, &p.VisitorName, &p.ValidFrom, &p.ValidUntil, &p.Status, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) RevokePreauth(ctx context.Context, complexID, hostID, id string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE visitor_vehicle_preauths SET status = 'revoked'
		 WHERE id = $1::uuid AND complex_id = $2::uuid AND host_user_id = $3::uuid AND status = 'active'`,
		id, complexID, hostID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrPreauthNotFound
	}
	return nil
}

func (r *Repository) LookupPreauth(ctx context.Context, complexID, plate string, now time.Time) (*domain.PreauthMatch, error) {
	var m domain.PreauthMatch
	err := r.db.QueryRowContext(ctx,
		`SELECT p.id::text, p.visitor_name, COALESCE(u.full_name, ''), COALESCE(un.name, ''), p.valid_until
		 FROM visitor_vehicle_preauths p
		 JOIN ref_users u ON u.id = p.host_user_id
		 LEFT JOIN ref_units un ON un.id = p.unit_id
		 WHERE p.complex_id = $1::uuid AND p.plate = $2 AND p.status = 'active'
		   AND p.valid_from <= $3 AND p.valid_until >= $3
		 ORDER BY p.valid_until DESC
		 LIMIT 1`,
		complexID, plate, now,
	).Scan(&m.ID, &m.VisitorName, &m.HostName, &m.UnitNumber, &m.ValidUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repository) LogAccess(ctx context.Context, entry *domain.AccessLog) error {
	var preauth any
	if entry.PreauthID != nil {
		preauth = *entry.PreauthID
	}
	return r.db.QueryRowContext(ctx,
		`INSERT INTO visitor_vehicle_access_log
		   (complex_id, plate, preauth_id, direction, guard_id, note)
		 VALUES ($1::uuid, $2, $3::uuid, $4, $5::uuid, $6)
		 RETURNING id::text, created_at`,
		entry.ComplexID, entry.Plate, preauth, entry.Direction, entry.GuardID, nullText(entry.Note),
	).Scan(&entry.ID, &entry.CreatedAt)
}

func (r *Repository) ListAccess(ctx context.Context, complexID string) ([]domain.AccessLog, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT l.id::text, l.complex_id::text, l.plate, l.preauth_id::text, l.direction, l.guard_id::text,
		        COALESCE(l.note, ''), l.created_at,
		        p.visitor_name, COALESCE(u.full_name, ''), COALESCE(un.name, '')
		 FROM visitor_vehicle_access_log l
		 LEFT JOIN visitor_vehicle_preauths p ON p.id = l.preauth_id
		 LEFT JOIN ref_users u ON u.id = p.host_user_id
		 LEFT JOIN ref_units un ON un.id = p.unit_id
		 WHERE l.complex_id = $1::uuid
		 ORDER BY l.created_at DESC
		 LIMIT 50`,
		complexID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AccessLog{}
	for rows.Next() {
		var entry domain.AccessLog
		var preauthID, visitorName sql.NullString
		var hostName, unitNumber string
		if err = rows.Scan(&entry.ID, &entry.ComplexID, &entry.Plate, &preauthID, &entry.Direction, &entry.GuardID, &entry.Note, &entry.CreatedAt, &visitorName, &hostName, &unitNumber); err != nil {
			return nil, err
		}
		if preauthID.Valid {
			id := preauthID.String
			entry.PreauthID = &id
			entry.VisitorPreauth = &domain.PreauthMatch{ID: id, VisitorName: visitorName.String, HostName: hostName, UnitNumber: unitNumber}
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}
