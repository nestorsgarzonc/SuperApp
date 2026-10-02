package postgres

import (
	"context"
	"database/sql"
	"errors"
	"superapp/auth/internal/domain"
)

func (r *Repository) Permissions(ctx context.Context) ([]domain.Permission, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,code,description FROM auth_permissions ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Permission{}
	for rows.Next() {
		var x domain.Permission
		if err = rows.Scan(&x.ID, &x.Code, &x.Description); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) CreatePermission(ctx context.Context, x domain.Permission) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO auth_permissions(id,code,description) VALUES($1,$2,$3)`, x.ID, x.Code, x.Description)
	return translateError(err)
}

func (r *Repository) PermissionByID(ctx context.Context, id string) (*domain.Permission, error) {
	x := &domain.Permission{}
	err := r.db.QueryRowContext(ctx, `SELECT id,code,description FROM auth_permissions WHERE id=$1`, id).Scan(&x.ID, &x.Code, &x.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return x, err
}

func (r *Repository) UpdatePermission(ctx context.Context, x domain.Permission) error {
	res, err := r.db.ExecContext(ctx, `UPDATE auth_permissions SET code=$2,description=$3 WHERE id=$1`, x.ID, x.Code, x.Description)
	if err != nil {
		return translateError(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Repository) DeletePermission(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM auth_permissions WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
