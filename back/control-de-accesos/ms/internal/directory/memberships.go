package directory

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

const (
	RoleResident = "RESIDENT"
	RoleGuard    = "GUARD"
	RoleAdmin    = "ADMIN"
)

// rolePriority decides which role wins when a person holds several in the same
// complex (e.g. resident and guard).
var rolePriority = map[string]int{RoleAdmin: 3, RoleGuard: 2, RoleResident: 1}

// HigherRole reports whether role a outranks role b.
func HigherRole(a, b string) bool { return rolePriority[a] > rolePriority[b] }

// Membership is one way a person belongs to a complex: as a resident of a unit
// or as staff.
type Membership struct {
	ComplexID   uuid.UUID  `json:"complex_id"`
	ComplexName string     `json:"complex_name"`
	Role        string     `json:"role"`
	UnitID      *uuid.UUID `json:"unit_id,omitempty"`
	UnitName    *string    `json:"unit_name,omitempty"`
}

const membershipsSQL = `
	SELECT u.complex_id, COALESCE(c.name, u.complex_id::text), 'RESIDENT', u.id, u.name
	FROM unit_members um
	JOIN units u ON u.id = um.unit_id
	LEFT JOIN complexes c ON c.id = u.complex_id
	WHERE um.user_id = $1
	UNION ALL
	SELECT cs.complex_id, COALESCE(c.name, cs.complex_id::text), cs.role, NULL::uuid, NULL::text
	FROM complex_staff cs
	LEFT JOIN complexes c ON c.id = cs.complex_id
	WHERE cs.user_id = $1
	ORDER BY 2, 3, 5`

// ListMemberships returns every complex the user belongs to, with the role and
// (for residents) the unit.
func (r *Repository) ListMemberships(ctx context.Context, userID uuid.UUID) ([]Membership, error) {
	rows, err := r.db.QueryContext(ctx, membershipsSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to load memberships: %w", err)
	}
	defer rows.Close()

	out := []Membership{}
	for rows.Next() {
		var (
			m        Membership
			unitID   uuid.NullUUID
			unitName sql.NullString
		)
		if err := rows.Scan(&m.ComplexID, &m.ComplexName, &m.Role, &unitID, &unitName); err != nil {
			return nil, fmt.Errorf("failed to scan membership: %w", err)
		}
		if unitID.Valid {
			m.UnitID = &unitID.UUID
			m.UnitName = &unitName.String
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetUserComplexRole returns the user's role in a complex (the highest one if
// they hold several). found is false when the user does not belong to it.
func (r *Repository) GetUserComplexRole(ctx context.Context, userID, complexID uuid.UUID) (role string, found bool, err error) {
	memberships, err := r.ListMemberships(ctx, userID)
	if err != nil {
		return "", false, err
	}
	for _, m := range memberships {
		if m.ComplexID != complexID {
			continue
		}
		if !found || HigherRole(m.Role, role) {
			role, found = m.Role, true
		}
	}
	return role, found, nil
}
