package access

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	CreateAccessGrant(ctx context.Context, complexID uuid.UUID, req CreateAccessGrantRequest, createdBy uuid.UUID) (*AccessGrant, error)
	GetAccessGrant(ctx context.Context, id uuid.UUID) (*AccessGrant, error)
	GetAccessGrantByQRHash(ctx context.Context, qrHash string) (*AccessGrant, error)
	GetUnitAccessGrants(ctx context.Context, unitID uuid.UUID, status *string) ([]*AccessGrant, error)
	GetComplexAccessGrants(ctx context.Context, complexID uuid.UUID, method *string, status *string) ([]*AccessGrant, error)
	// GetComplexAccessGrantsForUser returns only the grants of units the user belongs to:
	// a grant's QR code / ID number is a credential, so nobody lists other units' grants.
	GetComplexAccessGrantsForUser(ctx context.Context, complexID, userID uuid.UUID, method string) ([]*AccessGrant, error)
	// CreateAccessGrantsTx inserts all grants or none.
	CreateAccessGrantsTx(ctx context.Context, complexID, createdBy uuid.UUID, reqs []CreateAccessGrantRequest) ([]*AccessGrant, error)
	// GrantSummary counts a complex's grants without exposing any of them.
	GrantSummary(ctx context.Context, complexID uuid.UUID) (*GrantSummary, error)
	GetActiveAccessGrantsByVisitorID(ctx context.Context, complexID uuid.UUID, visitorIDNumber string) ([]*AccessGrant, error)
	CancelAccessGrant(ctx context.Context, id uuid.UUID) error
	// TryMarkGrantUsed atomically transitions an ACTIVE grant to USED.
	// Returns false if another caller already consumed it first.
	TryMarkGrantUsed(ctx context.Context, id uuid.UUID, usedBy uuid.UUID) (bool, error)
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const accessGrantColumns = `id, complex_id, unit_id, created_by, method, visitor_type, visitor_name,
	       visitor_id_number, qr_code_hash, details, status, valid_from, valid_until,
	       used_at, used_by, created_at, single_use`

func scanAccessGrant(row interface{ Scan(dest ...any) error }, grant *AccessGrant) error {
	return row.Scan(
		&grant.ID,
		&grant.ComplexID,
		&grant.UnitID,
		&grant.CreatedBy,
		&grant.Method,
		&grant.VisitorType,
		&grant.VisitorName,
		&grant.VisitorIDNumber,
		&grant.QRCodeHash,
		&grant.Details,
		&grant.Status,
		&grant.ValidFrom,
		&grant.ValidUntil,
		&grant.UsedAt,
		&grant.UsedBy,
		&grant.CreatedAt,
		&grant.SingleUse,
	)
}

type rowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func insertGrant(ctx context.Context, q rowQueryer, complexID uuid.UUID, req CreateAccessGrantRequest, createdBy uuid.UUID) (*AccessGrant, error) {
	query := `
		INSERT INTO access_grants (
			id, complex_id, unit_id, created_by, method, visitor_type, visitor_name,
			visitor_id_number, qr_code_hash, details, status, valid_from, valid_until, single_use
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING ` + accessGrantColumns

	validFrom := time.Now()
	if req.ValidFrom != nil {
		validFrom = *req.ValidFrom
	}

	grantID := uuid.New()
	var qrHash *string
	if req.Method == MethodQR {
		hashString := req.qrCode
		if hashString == "" { // no QR system wired (tests): derive locally
			hashInput := fmt.Sprintf("%s:%s:%d", grantID.String(), complexID.String(), validFrom.Unix())
			hash := sha256.Sum256([]byte(hashInput))
			hashString = hex.EncodeToString(hash[:])
		}
		qrHash = &hashString
	}

	var grant AccessGrant
	// Every grant is single-use: consumed on first successful validation
	// (see service.ValidateAccess / TryMarkGrantUsed below).
	row := q.QueryRowContext(
		ctx, query,
		grantID, complexID, req.UnitID, createdBy, req.Method, req.VisitorType, req.VisitorName,
		req.VisitorIDNumber, qrHash, req.Details, StatusActive, validFrom, req.ValidUntil, true,
	)
	if err := scanAccessGrant(row, &grant); err != nil {
		return nil, fmt.Errorf("failed to create access grant: %w", err)
	}
	return &grant, nil
}

func (r *PostgresRepository) CreateAccessGrant(ctx context.Context, complexID uuid.UUID, req CreateAccessGrantRequest, createdBy uuid.UUID) (*AccessGrant, error) {
	return insertGrant(ctx, r.db, complexID, req, createdBy)
}

func (r *PostgresRepository) CreateAccessGrantsTx(ctx context.Context, complexID, createdBy uuid.UUID, reqs []CreateAccessGrantRequest) ([]*AccessGrant, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	grants := make([]*AccessGrant, 0, len(reqs))
	for i, req := range reqs {
		g, err := insertGrant(ctx, tx, complexID, req, createdBy)
		if err != nil {
			return nil, fmt.Errorf("grant %d of %d: %w", i+1, len(reqs), err)
		}
		grants = append(grants, g)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit grants: %w", err)
	}
	return grants, nil
}

func (r *PostgresRepository) GetComplexAccessGrantsForUser(ctx context.Context, complexID, userID uuid.UUID, method string) ([]*AccessGrant, error) {
	return r.queryAccessGrants(ctx, `SELECT `+accessGrantColumns+` FROM access_grants
		WHERE complex_id = $1 AND method = $2
		  AND unit_id IN (SELECT um.unit_id FROM unit_members um WHERE um.user_id = $3)
		ORDER BY created_at DESC LIMIT 1000`, complexID, method, userID)
}

func (r *PostgresRepository) GrantSummary(ctx context.Context, complexID uuid.UUID) (*GrantSummary, error) {
	var s GrantSummary
	err := r.db.QueryRowContext(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'ACTIVE' AND valid_from <= NOW() AND valid_until >= NOW()),
		       count(*) FILTER (WHERE method = 'QR'),
		       count(*) FILTER (WHERE method = 'PRE_AUTH')
		FROM access_grants WHERE complex_id = $1`, complexID).Scan(&s.Total, &s.Active, &s.QR, &s.PreAuth)
	if err != nil {
		return nil, fmt.Errorf("failed to summarize grants: %w", err)
	}
	return &s, nil
}

func (r *PostgresRepository) GetAccessGrant(ctx context.Context, id uuid.UUID) (*AccessGrant, error) {
	query := `SELECT ` + accessGrantColumns + ` FROM access_grants WHERE id = $1`

	var grant AccessGrant
	row := r.db.QueryRowContext(ctx, query, id)
	if err := scanAccessGrant(row, &grant); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrAccessGrantNotFound
		}
		return nil, fmt.Errorf("failed to get access grant: %w", err)
	}

	return &grant, nil
}

func (r *PostgresRepository) GetAccessGrantByQRHash(ctx context.Context, qrHash string) (*AccessGrant, error) {
	query := `SELECT ` + accessGrantColumns + `
		FROM access_grants
		WHERE qr_code_hash = $1`

	var grant AccessGrant
	row := r.db.QueryRowContext(ctx, query, qrHash)
	if err := scanAccessGrant(row, &grant); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrAccessGrantNotFound
		}
		return nil, fmt.Errorf("failed to get access grant by QR: %w", err)
	}

	return &grant, nil
}

func (r *PostgresRepository) GetUnitAccessGrants(ctx context.Context, unitID uuid.UUID, status *string) ([]*AccessGrant, error) {
	query := `SELECT ` + accessGrantColumns + ` FROM access_grants WHERE unit_id = $1`

	args := []interface{}{unitID}
	if status != nil {
		query += ` AND status = $2`
		args = append(args, *status)
	}
	query += ` ORDER BY created_at DESC LIMIT 100`

	return r.queryAccessGrants(ctx, query, args...)
}

func (r *PostgresRepository) GetComplexAccessGrants(ctx context.Context, complexID uuid.UUID, method *string, status *string) ([]*AccessGrant, error) {
	query := `SELECT ` + accessGrantColumns + ` FROM access_grants WHERE complex_id = $1`

	args := []interface{}{complexID}
	argIndex := 2

	if method != nil {
		query += fmt.Sprintf(` AND method = $%d`, argIndex)
		args = append(args, *method)
		argIndex++
	}
	if status != nil {
		query += fmt.Sprintf(` AND status = $%d`, argIndex)
		args = append(args, *status)
	}
	query += ` ORDER BY created_at DESC LIMIT 1000`

	return r.queryAccessGrants(ctx, query, args...)
}

func (r *PostgresRepository) GetActiveAccessGrantsByVisitorID(ctx context.Context, complexID uuid.UUID, visitorIDNumber string) ([]*AccessGrant, error) {
	query := `SELECT ` + accessGrantColumns + `
		FROM access_grants
		WHERE complex_id = $1 AND visitor_id_number = $2 AND status = $3
		ORDER BY created_at DESC`

	return r.queryAccessGrants(ctx, query, complexID, visitorIDNumber, StatusActive)
}

func (r *PostgresRepository) queryAccessGrants(ctx context.Context, query string, args ...interface{}) ([]*AccessGrant, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query access grants: %w", err)
	}
	defer rows.Close()

	grants := []*AccessGrant{}
	for rows.Next() {
		var grant AccessGrant
		if err := scanAccessGrant(rows, &grant); err != nil {
			return nil, fmt.Errorf("failed to scan access grant: %w", err)
		}
		grants = append(grants, &grant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return grants, nil
}

func (r *PostgresRepository) CancelAccessGrant(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE access_grants SET status = $2 WHERE id = $1 AND status = $3`

	result, err := r.db.ExecContext(ctx, query, id, StatusCancelled, StatusActive)
	if err != nil {
		return fmt.Errorf("failed to cancel access grant: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return ErrAccessGrantNotFound
	}

	return nil
}

func (r *PostgresRepository) TryMarkGrantUsed(ctx context.Context, id uuid.UUID, usedBy uuid.UUID) (bool, error) {
	query := `
		UPDATE access_grants
		SET status = $2, used_at = NOW(), used_by = $3
		WHERE id = $1 AND status = $4
	`
	res, err := r.db.ExecContext(ctx, query, id, StatusUsed, usedBy, StatusActive)
	if err != nil {
		return false, fmt.Errorf("failed to mark grant used: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read rows affected: %w", err)
	}
	return rows > 0, nil
}
