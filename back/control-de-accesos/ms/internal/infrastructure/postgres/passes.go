package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"superapp/control-de-accesos/internal/domain"
)

func (r *Repository) CreatePass(ctx context.Context, p *domain.Pass) error {
	created, err := scanPass(r.db.QueryRowContext(ctx,
		`INSERT INTO visitor_passes
		   (complex_id, unit_id, host_user_id, visitor_name, document, token, valid_from, valid_until, status)
		 VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, 'active')
		 RETURNING `+passColumns,
		p.ComplexID, p.UnitID, p.HostUserID, p.VisitorName, nullText(p.Document), p.Token, p.ValidFrom, p.ValidUntil,
	))
	if err != nil {
		return err
	}
	*p = *created
	return nil
}

func (r *Repository) GetPass(ctx context.Context, complexID, id string) (*domain.Pass, error) {
	p, err := scanPass(r.db.QueryRowContext(ctx,
		`SELECT `+passColumns+` FROM visitor_passes WHERE id = $1::uuid AND complex_id = $2::uuid`,
		id, complexID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return p, err
}

func (r *Repository) ListPasses(ctx context.Context, complexID, unitID string, limit int) ([]domain.Pass, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+passColumns+`
		 FROM visitor_passes
		 WHERE complex_id = $1::uuid AND unit_id = $2::uuid
		 ORDER BY created_at DESC
		 LIMIT $3`,
		complexID, unitID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []domain.Pass{}
	for rows.Next() {
		p, err := scanPass(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *p)
	}
	return list, rows.Err()
}

func (r *Repository) PassByToken(ctx context.Context, complexID, token string) (*domain.Pass, error) {
	p, err := scanPass(r.db.QueryRowContext(ctx,
		`SELECT `+passColumns+` FROM visitor_passes WHERE token = $1 AND complex_id = $2::uuid`,
		token, complexID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrInvalidToken
	}
	return p, err
}

func (r *Repository) MarkUsed(ctx context.Context, complexID, id, guardID string, now time.Time) (*domain.Pass, error) {
	p, err := scanPass(r.db.QueryRowContext(ctx,
		`UPDATE visitor_passes
		 SET status = 'used', used_by = $1::uuid, used_at = $2
		 WHERE id = $3::uuid AND complex_id = $4::uuid
		   AND status = 'active' AND valid_from <= $2 AND valid_until > $2
		 RETURNING `+passColumns,
		guardID, now, id, complexID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrPassNotActive
	}
	return p, err
}

func (r *Repository) RevokePass(ctx context.Context, complexID, id string) (*domain.Pass, error) {
	p, err := scanPass(r.db.QueryRowContext(ctx,
		`UPDATE visitor_passes SET status = 'revoked'
		 WHERE id = $1::uuid AND complex_id = $2::uuid AND status = 'active'
		 RETURNING `+passColumns,
		id, complexID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrPassNotActive
	}
	return p, err
}

func (r *Repository) ExtendPass(ctx context.Context, complexID, id string, validUntil, now time.Time) (*domain.Pass, error) {
	p, err := scanPass(r.db.QueryRowContext(ctx,
		`UPDATE visitor_passes SET valid_until = $1
		 WHERE id = $2::uuid AND complex_id = $3::uuid
		   AND status = 'active' AND valid_until > $4 AND valid_until < $1
		 RETURNING `+passColumns,
		validUntil, id, complexID, now,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrPassNotExtendable
	}
	return p, err
}

func (r *Repository) MarkCheckedOut(ctx context.Context, complexID, id, guardID string, now time.Time) (*domain.Pass, error) {
	p, err := scanPass(r.db.QueryRowContext(ctx,
		`UPDATE visitor_passes
		 SET status = 'checked_out', checked_out_by = $1::uuid, checked_out_at = $2
		 WHERE id = $3::uuid AND complex_id = $4::uuid AND status = 'used'
		 RETURNING `+passColumns,
		guardID, now, id, complexID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrPassNotCheckedIn
	}
	return p, err
}

func (r *Repository) SweepExpired(ctx context.Context, now time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE visitor_passes SET status = 'expired' WHERE status = 'active' AND valid_until <= $1`,
		now,
	)
	return err
}

func (r *Repository) Checkpoint(ctx context.Context, complexID string, limit int, now time.Time) (*domain.CheckpointFeed, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH checkpoint_events AS (
			SELECT vp.id::text AS pass_id, 'in'::text AS direction,
			       CONCAT_WS(' - ', NULLIF(s.name, ''), u.name) AS unit_label,
			       vp.used_at AS occurred_at
			FROM visitor_passes vp
			JOIN ref_units u ON u.id = vp.unit_id
			LEFT JOIN ref_sections s ON s.id = u.section_id
			WHERE vp.complex_id = $1::uuid AND vp.used_at IS NOT NULL
			UNION ALL
			SELECT vp.id::text, 'out'::text,
			       CONCAT_WS(' - ', NULLIF(s.name, ''), u.name),
			       vp.checked_out_at
			FROM visitor_passes vp
			JOIN ref_units u ON u.id = vp.unit_id
			LEFT JOIN ref_sections s ON s.id = u.section_id
			WHERE vp.complex_id = $1::uuid AND vp.checked_out_at IS NOT NULL
		)
		SELECT pass_id, direction, unit_label, occurred_at
		FROM checkpoint_events
		ORDER BY occurred_at DESC
		LIMIT $2`, complexID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	activities := []domain.CheckpointActivity{}
	for rows.Next() {
		var item domain.CheckpointActivity
		if err = rows.Scan(&item.PassID, &item.Direction, &item.UnitLabel, &item.OccurredAt); err != nil {
			return nil, err
		}
		activities = append(activities, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	feed := &domain.CheckpointFeed{Activities: activities, GeneratedAt: now}
	err = r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE used_at >= $2),
			COUNT(*) FILTER (WHERE checked_out_at >= $2),
			COUNT(*) FILTER (WHERE status = 'used')
		FROM visitor_passes
		WHERE complex_id = $1::uuid`, complexID, now.Add(-24*time.Hour)).Scan(
		&feed.EntriesLast24h, &feed.ExitsLast24h, &feed.CurrentlyInside,
	)
	if err != nil {
		return nil, err
	}
	return feed, nil
}

func (r *Repository) Notify(ctx context.Context, complexID, userID, title, body, kind string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO integration_notifications (complex_id, user_id, title, body, type)
		 VALUES ($1::uuid, $2::uuid, $3, $4, $5)`,
		complexID, userID, title, body, kind,
	)
	return err
}
