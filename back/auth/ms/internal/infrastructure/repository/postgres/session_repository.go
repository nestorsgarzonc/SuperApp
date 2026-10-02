package postgres

import (
	"context"
	"database/sql"
	"errors"
	"superapp/auth/internal/core/domain"
	coreRepository "superapp/auth/internal/core/ports/repository"
	"time"
)

func (r *Repository) CreateSession(ctx context.Context, params coreRepository.CreateSessionParams) error {
	tokenID, err := newUUID()
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO auth_sessions(id,user_id,device_name,user_agent,ip_address,expires_at) VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,'')::inet,$6)`, params.SessionID, params.UserID, params.Device, params.UserAgent, params.IP, params.ExpiresAt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO auth_refresh_tokens(id,session_id,token_hash,expires_at) VALUES($1,$2,decode($3,'hex'),$4)`, tokenID, params.SessionID, params.RefreshHash, params.ExpiresAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) RotateRefresh(ctx context.Context, params coreRepository.RotateRefreshParams) (coreRepository.RotateRefreshResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return coreRepository.RotateRefreshResult{}, err
	}
	defer tx.Rollback()
	var sid, uid, email, status string
	var oldID string
	var oldExpiry time.Time
	var consumed, revoked, sessionRevoked sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT t.id,t.session_id,s.user_id,u.email,u.status,t.expires_at,t.consumed_at,t.revoked_at,s.revoked_at FROM auth_refresh_tokens t JOIN auth_sessions s ON s.id=t.session_id JOIN auth_users u ON u.id=s.user_id WHERE t.token_hash=decode($1,'hex') FOR UPDATE OF t,s`, params.OldHash).Scan(&oldID, &sid, &uid, &email, &status, &oldExpiry, &consumed, &revoked, &sessionRevoked)
	if errors.Is(err, sql.ErrNoRows) {
		return coreRepository.RotateRefreshResult{}, domain.ErrInvalidToken
	}
	if err != nil {
		return coreRepository.RotateRefreshResult{}, err
	}
	if consumed.Valid || revoked.Valid || sessionRevoked.Valid || !oldExpiry.After(params.Now) || status != "ACTIVE" {
		if consumed.Valid || revoked.Valid {
			_, err = tx.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,$2) WHERE id=$1`, sid, params.Now)
			if err != nil {
				return coreRepository.RotateRefreshResult{}, err
			}
			_, err = tx.ExecContext(ctx, `UPDATE auth_refresh_tokens SET revoked_at=COALESCE(revoked_at,$2) WHERE session_id=$1`, sid, params.Now)
			if err != nil {
				return coreRepository.RotateRefreshResult{}, err
			}
		}
		if err = tx.Commit(); err != nil {
			return coreRepository.RotateRefreshResult{}, err
		}
		return coreRepository.RotateRefreshResult{}, domain.ErrInvalidToken
	}
	_, err = tx.ExecContext(ctx, `UPDATE auth_refresh_tokens SET consumed_at=$2 WHERE id=$1`, oldID, params.Now)
	if err != nil {
		return coreRepository.RotateRefreshResult{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO auth_refresh_tokens(id,session_id,token_hash,expires_at) VALUES($1,$2,decode($3,'hex'),$4)`, params.NewTokenID, sid, params.NewHash, params.ExpiresAt)
	if err != nil {
		return coreRepository.RotateRefreshResult{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE auth_refresh_tokens SET replaced_by=$2 WHERE id=$1`, oldID, params.NewTokenID)
	if err != nil {
		return coreRepository.RotateRefreshResult{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE auth_sessions SET expires_at=$2 WHERE id=$1`, sid, params.ExpiresAt)
	if err != nil {
		return coreRepository.RotateRefreshResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return coreRepository.RotateRefreshResult{}, err
	}
	return coreRepository.RotateRefreshResult{SessionID: sid, UserID: uid, Email: email}, nil
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

func (r *Repository) RevokeSession(ctx context.Context, params coreRepository.RevokeSessionParams) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1 AND user_id=$2`, params.SessionID, params.UserID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `UPDATE auth_refresh_tokens SET revoked_at=COALESCE(revoked_at,now()) WHERE session_id=$1`, params.SessionID); err != nil {
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
