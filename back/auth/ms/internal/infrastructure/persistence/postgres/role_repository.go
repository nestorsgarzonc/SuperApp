package postgres

import (
	"context"
	"database/sql"
	"errors"
	"superapp/auth/internal/domain"
)

func (r *Repository) Roles(ctx context.Context, tenant *string) ([]domain.Role, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,code,name,description,scope,tenant_id,enabled FROM auth_roles WHERE (scope='SYSTEM' AND tenant_id IS NULL) OR (scope='TENANT' AND tenant_id=$1) ORDER BY scope,code`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Role{}
	for rows.Next() {
		var x domain.Role
		if err = rows.Scan(&x.ID, &x.Code, &x.Name, &x.Description, &x.Scope, &x.TenantID, &x.Enabled); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) CreateRole(ctx context.Context, x domain.Role) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO auth_roles(id,code,name,description,scope,tenant_id,enabled) VALUES($1,$2,$3,$4,$5,$6,$7)`, x.ID, x.Code, x.Name, x.Description, x.Scope, x.TenantID, x.Enabled)
	return translateError(err)
}

func (r *Repository) RoleByID(ctx context.Context, id string, tenant *string) (*domain.Role, error) {
	x := &domain.Role{}
	err := r.db.QueryRowContext(ctx, `SELECT id,code,name,description,scope,tenant_id,enabled FROM auth_roles WHERE id=$1 AND ((scope='SYSTEM' AND tenant_id IS NULL) OR (scope='TENANT' AND tenant_id=$2))`, id, tenant).Scan(&x.ID, &x.Code, &x.Name, &x.Description, &x.Scope, &x.TenantID, &x.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return x, err
}

func (r *Repository) UpdateRole(ctx context.Context, x domain.Role, tenant *string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE auth_roles SET name=$3,description=$4,enabled=$5 WHERE id=$1 AND tenant_id IS NOT DISTINCT FROM $2`, x.ID, tenant, x.Name, x.Description, x.Enabled)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Repository) DeleteRole(ctx context.Context, id string, tenant *string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM auth_roles WHERE id=$1 AND tenant_id IS NOT DISTINCT FROM $2`, id, tenant)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
