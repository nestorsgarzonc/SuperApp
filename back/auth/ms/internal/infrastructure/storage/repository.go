package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"superapp/auth/internal/domain"
)

type Repository struct{ db *sql.DB }

func New(db *sql.DB) *Repository                     { return &Repository{db: db} }
func (r *Repository) Ping(ctx context.Context) error { return r.db.PingContext(ctx) }

func (r *Repository) CreateUser(ctx context.Context, id, email, hash, name string) (*domain.User, error) {
	u := &domain.User{}
	err := r.db.QueryRowContext(ctx, `INSERT INTO auth_users(id,email,email_normalized,password_hash,full_name) VALUES($1,$2,$2,$3,$4) ON CONFLICT(email_normalized) DO NOTHING RETURNING id,email,full_name,status,created_at`, id, email, hash, name).Scan(&u.ID, &u.Email, &u.FullName, &u.Status, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrEmailTaken
	}
	return u, err
}
func (r *Repository) UserByEmail(ctx context.Context, email string) (*domain.User, string, error) {
	u := &domain.User{}
	var hash string
	err := r.db.QueryRowContext(ctx, `SELECT id,email,full_name,status,created_at,password_hash FROM auth_users WHERE email_normalized=$1`, email).Scan(&u.ID, &u.Email, &u.FullName, &u.Status, &u.CreatedAt, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	return u, hash, err
}
func (r *Repository) UserByID(ctx context.Context, id string) (*domain.User, error) {
	u := &domain.User{}
	err := r.db.QueryRowContext(ctx, `SELECT id,email,full_name,status,created_at FROM auth_users WHERE id=$1`, id).Scan(&u.ID, &u.Email, &u.FullName, &u.Status, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return u, err
}
func (r *Repository) UpdateUser(ctx context.Context, id, email, name string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE auth_users SET email=$2,email_normalized=$2,full_name=$3,updated_at=now() WHERE id=$1`, id, email, name)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrEmailTaken
		}
		return translateError(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
func (r *Repository) SetLastLogin(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE auth_users SET last_login_at=now() WHERE id=$1`, id)
	return err
}
func (r *Repository) CreateSession(ctx context.Context, sid, uid, tokenHash string, expires time.Time, device, ua, ip string) error {
	tokenID, err := newUUID()
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO auth_sessions(id,user_id,device_name,user_agent,ip_address,expires_at) VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,'')::inet,$6)`, sid, uid, device, ua, ip, expires)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO auth_refresh_tokens(id,session_id,token_hash,expires_at) VALUES($1,$2,decode($3,'hex'),$4)`, tokenID, sid, tokenHash, expires)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) RotateRefresh(ctx context.Context, oldHash, newHash, newTokenID string, expires, now time.Time) (string, string, string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", "", err
	}
	defer tx.Rollback()
	var sid, uid, email, status string
	var oldID string
	var oldExpiry time.Time
	var consumed, revoked, sessionRevoked sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT t.id,t.session_id,s.user_id,u.email,u.status,t.expires_at,t.consumed_at,t.revoked_at,s.revoked_at FROM auth_refresh_tokens t JOIN auth_sessions s ON s.id=t.session_id JOIN auth_users u ON u.id=s.user_id WHERE t.token_hash=decode($1,'hex') FOR UPDATE OF t,s`, oldHash).Scan(&oldID, &sid, &uid, &email, &status, &oldExpiry, &consumed, &revoked, &sessionRevoked)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", domain.ErrInvalidToken
	}
	if err != nil {
		return "", "", "", err
	}
	if consumed.Valid || revoked.Valid || sessionRevoked.Valid || !oldExpiry.After(now) || status != "ACTIVE" {
		if consumed.Valid || revoked.Valid {
			_, err = tx.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,$2) WHERE id=$1`, sid, now)
			if err != nil {
				return "", "", "", err
			}
			_, err = tx.ExecContext(ctx, `UPDATE auth_refresh_tokens SET revoked_at=COALESCE(revoked_at,$2) WHERE session_id=$1`, sid, now)
			if err != nil {
				return "", "", "", err
			}
		}
		if err = tx.Commit(); err != nil {
			return "", "", "", err
		}
		return "", "", "", domain.ErrInvalidToken
	}
	_, err = tx.ExecContext(ctx, `UPDATE auth_refresh_tokens SET consumed_at=$2 WHERE id=$1`, oldID, now)
	if err != nil {
		return "", "", "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO auth_refresh_tokens(id,session_id,token_hash,expires_at) VALUES($1,$2,decode($3,'hex'),$4)`, newTokenID, sid, newHash, expires)
	if err != nil {
		return "", "", "", err
	}
	_, err = tx.ExecContext(ctx, `UPDATE auth_refresh_tokens SET replaced_by=$2 WHERE id=$1`, oldID, newTokenID)
	if err != nil {
		return "", "", "", err
	}
	_, err = tx.ExecContext(ctx, `UPDATE auth_sessions SET expires_at=$2 WHERE id=$1`, sid, expires)
	if err != nil {
		return "", "", "", err
	}
	if err = tx.Commit(); err != nil {
		return "", "", "", err
	}
	return sid, uid, email, nil
}
func (r *Repository) RevokeRefresh(ctx context.Context, hash string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE auth_sessions s SET revoked_at=COALESCE(s.revoked_at,now()) FROM auth_refresh_tokens t WHERE t.session_id=s.id AND t.token_hash=decode($1,'hex')`, hash)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE auth_refresh_tokens SET revoked_at=COALESCE(revoked_at,now()) WHERE token_hash=decode($1,'hex')`, hash)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) RevokeAllSessions(ctx context.Context, uid string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`, uid)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE auth_refresh_tokens t SET revoked_at=COALESCE(t.revoked_at,now()) FROM auth_sessions s WHERE t.session_id=s.id AND s.user_id=$1`, uid)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) RevokeSession(ctx context.Context, uid, sid string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1 AND user_id=$2`, sid, uid)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `UPDATE auth_refresh_tokens SET revoked_at=COALESCE(revoked_at,now()) WHERE session_id=$1`, sid); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *Repository) Sessions(ctx context.Context, uid string) ([]domain.Session, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,COALESCE(device_name,''),created_at,expires_at FROM auth_sessions WHERE user_id=$1 AND revoked_at IS NULL AND expires_at>now() ORDER BY created_at DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Session{}
	for rows.Next() {
		var s domain.Session
		if err = rows.Scan(&s.ID, &s.Device, &s.CreatedAt, &s.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
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
func (r *Repository) SetUserStatus(ctx context.Context, user, status string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE auth_users SET status=$2,updated_at=now() WHERE id=$1`, user, status)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	if status != "ACTIVE" {
		_, err = tx.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`, user)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE auth_refresh_tokens t SET revoked_at=COALESCE(t.revoked_at,now()) FROM auth_sessions s WHERE t.session_id=s.id AND s.user_id=$1`, user)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
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

func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func translateError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return domain.ErrConflict
		case "23503":
			return domain.ErrNotFound
		case "23514", "22P02":
			return domain.ErrInvalidArgument
		}
	}
	return err
}
