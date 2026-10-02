// Package directory holds this microservice's own local read-model of who
// belongs to which unit/complex, since it doesn't share a database with
// whatever service owns that data.
package directory

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// GetUnitComplexID returns the complex a unit belongs to.
func (r *Repository) GetUnitComplexID(ctx context.Context, unitID uuid.UUID) (uuid.UUID, error) {
	var complexID uuid.UUID
	err := r.db.QueryRowContext(ctx, `SELECT complex_id FROM units WHERE id = $1`, unitID).Scan(&complexID)
	if err == sql.ErrNoRows {
		return uuid.Nil, fmt.Errorf("unit not found")
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to load unit: %w", err)
	}
	return complexID, nil
}

// GetUnitName returns a unit's display name, used to enrich QR responses.
func (r *Repository) GetUnitName(ctx context.Context, unitID uuid.UUID) (string, error) {
	var name string
	err := r.db.QueryRowContext(ctx, `SELECT name FROM units WHERE id = $1`, unitID).Scan(&name)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("unit not found")
	}
	if err != nil {
		return "", fmt.Errorf("failed to load unit name: %w", err)
	}
	return name, nil
}

// GetUserDisplayName returns a user's display name, used to enrich QR responses.
func (r *Repository) GetUserDisplayName(ctx context.Context, userID uuid.UUID) (string, error) {
	var name string
	err := r.db.QueryRowContext(ctx, `SELECT name FROM users WHERE id = $1`, userID).Scan(&name)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("user not found")
	}
	if err != nil {
		return "", fmt.Errorf("failed to load user: %w", err)
	}
	return name, nil
}

// IsUserAssociatedWithUnit reports whether the user is a member of the unit.
func (r *Repository) IsUserAssociatedWithUnit(ctx context.Context, userID, unitID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM unit_members WHERE user_id = $1 AND unit_id = $2)`,
		userID, unitID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check unit membership: %w", err)
	}
	return exists, nil
}

// IsUserAssociatedWithComplex reports whether the user is a member of any
// unit belonging to the complex.
func (r *Repository) IsUserAssociatedWithComplex(ctx context.Context, userID, complexID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM unit_members um
			JOIN units u ON u.id = um.unit_id
			WHERE um.user_id = $1 AND u.complex_id = $2
		)`,
		userID, complexID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check complex membership: %w", err)
	}
	return exists, nil
}
