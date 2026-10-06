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
func (r *Repository) GetUnitComplexID(ctx context.Context, unitID uuid.UUID) (uuid.UUID, bool, error) {
	var complexID uuid.UUID
	err := r.db.QueryRowContext(ctx, `SELECT complex_id FROM units WHERE id = $1`, unitID).Scan(&complexID)
	if err == sql.ErrNoRows {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("failed to load unit: %w", err)
	}
	return complexID, true, nil
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

// GetUnitMemberIDs returns the users that belong to the unit (its residents),
// used to address notifications.
func (r *Repository) GetUnitMemberIDs(ctx context.Context, unitID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT user_id FROM unit_members WHERE unit_id = $1`, unitID)
	if err != nil {
		return nil, fmt.Errorf("failed to load unit members: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan unit member: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to load unit members: %w", err)
	}
	return ids, nil
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

// IsUserAssociatedWithComplex reports whether the user belongs to the complex:
// as a member of one of its units (resident) or as its staff (guard/admin).
func (r *Repository) IsUserAssociatedWithComplex(ctx context.Context, userID, complexID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM unit_members um
			JOIN units u ON u.id = um.unit_id
			WHERE um.user_id = $1 AND u.complex_id = $2
		) OR EXISTS(
			SELECT 1 FROM complex_staff WHERE user_id = $1 AND complex_id = $2
		)`,
		userID, complexID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check complex membership: %w", err)
	}
	return exists, nil
}

// IsStaffOfComplex reports whether the user is registered as GUARD/ADMIN
// (role) of exactly this complex. A role claim in a token is never enough on
// its own: being a guard of complex 1 says nothing about complex 2.
func (r *Repository) IsStaffOfComplex(ctx context.Context, userID, complexID uuid.UUID, role string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM complex_staff WHERE user_id = $1 AND complex_id = $2 AND role = $3)`,
		userID, complexID, role,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check staff membership: %w", err)
	}
	return exists, nil
}
