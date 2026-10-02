package postgres

import (
	"context"
	"superapp/auth/internal/core/domain"
)

func (r *Repository) GrantPermission(ctx context.Context, role, permission string, tenant *string) error {
	res, err := r.db.ExecContext(ctx, `INSERT INTO auth_role_permissions(role_id,permission_id) SELECT r.id,p.id FROM auth_roles r CROSS JOIN auth_permissions p WHERE r.id=$1 AND p.id=$2 AND r.tenant_id IS NOT DISTINCT FROM $3 ON CONFLICT DO NOTHING`, role, permission, tenant)
	if err != nil {
		return translateError(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		var exists bool
		if err = r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM auth_role_permissions rp JOIN auth_roles r ON r.id=rp.role_id WHERE rp.role_id=$1 AND rp.permission_id=$2 AND r.tenant_id IS NOT DISTINCT FROM $3)`, role, permission, tenant).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil
		}
		return domain.ErrNotFound
	}
	return nil
}

func (r *Repository) RevokePermission(ctx context.Context, role, permission string, tenant *string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM auth_role_permissions rp USING auth_roles r WHERE rp.role_id=r.id AND r.id=$1 AND rp.permission_id=$2 AND r.tenant_id IS NOT DISTINCT FROM $3`, role, permission, tenant)
	return err
}

func (r *Repository) AssignRole(ctx context.Context, user, role string, tenant *string, actor string) error {
	id, err := newUUID()
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO auth_user_roles(id,user_id,role_id,tenant_id,assigned_by) SELECT $1,$2,r.id,$3,NULLIF($4,'') FROM auth_roles r WHERE r.id=$5 AND r.enabled AND ((r.scope='SYSTEM' AND r.tenant_id IS NULL AND $3::uuid IS NULL) OR (r.scope='TENANT' AND r.tenant_id=$3)) ON CONFLICT DO NOTHING`, id, user, tenant, actor, role)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (r *Repository) RevokeRole(ctx context.Context, user, role string, tenant *string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE auth_user_roles SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1 AND role_id=$2 AND tenant_id IS NOT DISTINCT FROM $3 AND revoked_at IS NULL`, user, role, tenant)
	return err
}

func (r *Repository) EffectiveRoles(ctx context.Context, user string, tenant *string) ([]domain.Role, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT r.id,r.code,r.name,r.description,r.scope,r.tenant_id,r.enabled FROM auth_user_roles ur JOIN auth_roles r ON r.id=ur.role_id WHERE ur.user_id=$1 AND ur.revoked_at IS NULL AND r.enabled AND ((ur.tenant_id IS NULL AND r.scope='SYSTEM' AND r.tenant_id IS NULL) OR (ur.tenant_id=$2 AND r.scope='TENANT' AND r.tenant_id=$2)) ORDER BY r.code`, user, tenant)
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

func (r *Repository) EffectivePermissions(ctx context.Context, user string, tenant *string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT p.code FROM auth_user_roles ur JOIN auth_roles r ON r.id=ur.role_id JOIN auth_role_permissions rp ON rp.role_id=r.id JOIN auth_permissions p ON p.id=rp.permission_id WHERE ur.user_id=$1 AND ur.revoked_at IS NULL AND r.enabled AND ((ur.tenant_id IS NULL AND r.scope='SYSTEM' AND r.tenant_id IS NULL) OR (ur.tenant_id=$2 AND r.scope='TENANT' AND r.tenant_id=$2)) ORDER BY p.code`, user, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var x string
		if err = rows.Scan(&x); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *Repository) PermissionAllowed(ctx context.Context, user, permission string, tenant *string) (bool, error) {
	var allowed bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM auth_user_roles ur JOIN auth_roles r ON r.id=ur.role_id JOIN auth_role_permissions rp ON rp.role_id=r.id JOIN auth_permissions p ON p.id=rp.permission_id WHERE ur.user_id=$1 AND p.code=$2 AND ur.revoked_at IS NULL AND r.enabled AND ((ur.tenant_id IS NULL AND r.scope='SYSTEM' AND r.tenant_id IS NULL) OR (ur.tenant_id=$3 AND r.scope='TENANT' AND r.tenant_id=$3)))`, user, permission, tenant).Scan(&allowed)
	return allowed, err
}
