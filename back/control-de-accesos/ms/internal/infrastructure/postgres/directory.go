package postgres

import (
	"context"

	"superapp/control-de-accesos/internal/domain"
)

func (r *Repository) Complexes(ctx context.Context) ([]domain.Complex, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id::text, name, address, timezone FROM ref_complexes WHERE is_active ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Complex{}
	for rows.Next() {
		var item domain.Complex
		if err = rows.Scan(&item.ID, &item.Name, &item.Address, &item.Timezone); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) People(ctx context.Context) ([]domain.Person, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT u.id::text, u.full_name, u.email,
		        CASE WHEN s.role = 'GUARD' THEN 'guard' ELSE 'resident' END AS role,
		        m.unit_id::text, un.name, sec.name
		 FROM ref_users u
		 LEFT JOIN ref_complex_staff s ON s.user_id = u.id AND s.role = 'GUARD'
		 LEFT JOIN ref_unit_members m ON m.user_id = u.id
		 LEFT JOIN ref_units un ON un.id = m.unit_id
		 LEFT JOIN ref_sections sec ON sec.id = un.section_id
		 WHERE s.user_id IS NOT NULL OR m.user_id IS NOT NULL
		 ORDER BY role, u.full_name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Person{}
	for rows.Next() {
		var item domain.Person
		if err = rows.Scan(&item.ID, &item.FullName, &item.Email, &item.Role, &item.UnitID, &item.UnitName, &item.SectionName); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) Notifications(ctx context.Context, complexID, userID string) ([]domain.Notification, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id::text, title, body, type, created_at
		 FROM integration_notifications
		 WHERE complex_id = $1::uuid AND user_id = $2::uuid
		 ORDER BY created_at DESC
		 LIMIT 20`,
		complexID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Notification{}
	for rows.Next() {
		var item domain.Notification
		if err = rows.Scan(&item.ID, &item.Title, &item.Body, &item.Type, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
