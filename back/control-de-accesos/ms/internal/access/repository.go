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
	GetActiveAccessGrantsByVisitorID(ctx context.Context, complexID uuid.UUID, visitorIDNumber string) ([]*AccessGrant, error)
	CancelAccessGrant(ctx context.Context, id uuid.UUID) error
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const accessGrantColumns = `id, complex_id, unit_id, created_by, method, visitor_type, visitor_name,
	       visitor_id_number, qr_code_hash, details, status, valid_from, valid_until,
	       used_at, used_by, created_at`

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
	)
}

func (r *PostgresRepository) CreateAccessGrant(ctx context.Context, complexID uuid.UUID, req CreateAccessGrantRequest, createdBy uuid.UUID) (*AccessGrant, error) {
	query := `
		INSERT INTO access_grants (
			id, complex_id, unit_id, created_by, method, visitor_type, visitor_name,
			visitor_id_number, qr_code_hash, details, status, valid_from, valid_until
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING ` + accessGrantColumns

	validFrom := time.Now()
	if req.ValidFrom != nil {
		validFrom = *req.ValidFrom
	}

	grantID := uuid.New()
	var qrHash *string
	if req.Method == MethodQR {
		hashInput := fmt.Sprintf("%s:%s:%d", grantID.String(), complexID.String(), validFrom.Unix())
		hash := sha256.Sum256([]byte(hashInput))
		hashString := hex.EncodeToString(hash[:])
		qrHash = &hashString
	}

	var grant AccessGrant
	row := r.db.QueryRowContext(
		ctx, query,
		grantID, complexID, req.UnitID, createdBy, req.Method, req.VisitorType, req.VisitorName,
		req.VisitorIDNumber, qrHash, req.Details, StatusActive, validFrom, req.ValidUntil,
	)
	if err := scanAccessGrant(row, &grant); err != nil {
		return nil, fmt.Errorf("failed to create access grant: %w", err)
	}

	return &grant, nil
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
		WHERE qr_code_hash = $1 AND status = $2 AND valid_from <= NOW() AND valid_until >= NOW()`

	var grant AccessGrant
	row := r.db.QueryRowContext(ctx, query, qrHash, StatusActive)
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
		      AND valid_from <= NOW() AND valid_until >= NOW()
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
