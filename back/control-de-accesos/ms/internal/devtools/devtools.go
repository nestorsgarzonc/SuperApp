// Package devtools holds development-only endpoints (mounted only when
// ENV=development) that stand in for back/auth while it isn't wired in:
//
//	GET /dev/users                         people and their memberships, read from the DB
//	GET /dev/token?user_id=&complex_id=    a JWT for that person acting in that complex
//
// Nothing here is hardcoded: complexes, units, people and roles come from the
// service's own directory tables, so adding a complex or a person to the seed
// is enough for them to show up in the front's dev login.
package devtools

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"controldeaccesos/internal/platform/errors"
	authmw "controldeaccesos/internal/platform/middleware"
	"controldeaccesos/internal/qrsystem"

	"github.com/google/uuid"
)

const (
	roleResident = "RESIDENT"
	roleGuard    = "GUARD"
	roleAdmin    = "ADMIN"
)

// rolePriority decides which membership wins when a person has several roles
// in the same complex and the caller didn't ask for one.
var rolePriority = map[string]int{roleAdmin: 3, roleGuard: 2, roleResident: 1}

type Membership struct {
	ComplexID   uuid.UUID  `json:"complex_id"`
	ComplexName string     `json:"complex_name"`
	Role        string     `json:"role"`
	UnitID      *uuid.UUID `json:"unit_id,omitempty"`
	UnitName    *string    `json:"unit_name,omitempty"`
}

type User struct {
	ID          uuid.UUID    `json:"user_id"`
	Name        string       `json:"name"`
	Memberships []Membership `json:"memberships"`
}

type Handler struct {
	db *sql.DB
	qr *qrsystem.System
}

func NewHandler(db *sql.DB, qr *qrsystem.System) *Handler { return &Handler{db: db, qr: qr} }

const membershipsSQL = `
	SELECT um.user_id, u.complex_id, COALESCE(c.name, u.complex_id::text), 'RESIDENT', u.id, u.name
	FROM unit_members um
	JOIN units u ON u.id = um.unit_id
	LEFT JOIN complexes c ON c.id = u.complex_id
	UNION ALL
	SELECT cs.user_id, cs.complex_id, COALESCE(c.name, cs.complex_id::text), cs.role, NULL::uuid, NULL::text
	FROM complex_staff cs
	LEFT JOIN complexes c ON c.id = cs.complex_id`

func (h *Handler) load(ctx context.Context, onlyUser *uuid.UUID) ([]User, error) {
	query := `SELECT usr.id, usr.name, m.complex_id, m.cname, m.role, m.unit_id, m.unit_name
		FROM users usr
		LEFT JOIN (SELECT * FROM (` + membershipsSQL + `) x(user_id, complex_id, cname, role, unit_id, unit_name)) m
			ON m.user_id = usr.id`
	args := []any{}
	if onlyUser != nil {
		query += ` WHERE usr.id = $1`
		args = append(args, *onlyUser)
	}
	query += ` ORDER BY usr.name, m.cname, m.role, m.unit_name`

	rows, err := h.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to load demo users: %w", err)
	}
	defer rows.Close()

	var users []User
	index := map[uuid.UUID]int{}
	for rows.Next() {
		var (
			id, name   = uuid.Nil, ""
			complexID  uuid.NullUUID
			cname, rol sql.NullString
			unitID     uuid.NullUUID
			unitName   sql.NullString
		)
		if err := rows.Scan(&id, &name, &complexID, &cname, &rol, &unitID, &unitName); err != nil {
			return nil, fmt.Errorf("failed to scan demo user: %w", err)
		}
		i, ok := index[id]
		if !ok {
			users = append(users, User{ID: id, Name: name, Memberships: []Membership{}})
			i = len(users) - 1
			index[id] = i
		}
		if !complexID.Valid {
			continue
		}
		m := Membership{ComplexID: complexID.UUID, ComplexName: cname.String, Role: rol.String}
		if unitID.Valid {
			m.UnitID = &unitID.UUID
			m.UnitName = &unitName.String
		}
		users[i].Memberships = append(users[i].Memberships, m)
	}
	return users, rows.Err()
}

// Users handles GET /api/v1/dev/users.
func (h *Handler) Users(w http.ResponseWriter, r *http.Request) {
	users, err := h.load(r.Context(), nil)
	if err != nil {
		errors.RespondError(w, nil, errors.NewInternalError(err))
		return
	}
	if users == nil {
		users = []User{}
	}
	errors.RespondOK(w, users)
}

type tokenResponse struct {
	Token       string       `json:"token"`
	UserID      uuid.UUID    `json:"user_id"`
	Name        string       `json:"name"`
	ComplexID   uuid.UUID    `json:"complex_id"`
	ComplexName string       `json:"complex_name"`
	Role        string       `json:"role"`
	UnitID      *uuid.UUID   `json:"unit_id,omitempty"`
	UnitName    *string      `json:"unit_name,omitempty"`
	Memberships []Membership `json:"memberships"`
}

// Token handles GET /api/v1/dev/token?user_id=&complex_id=[&role=].
//
// The role and unit are derived from the person's real membership in that
// complex — a token can't be minted for a complex the person doesn't belong
// to, nor with a role they don't hold there. `role` only disambiguates when
// the person holds several roles in the same complex.
func (h *Handler) Token(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	userID, err := uuid.Parse(q.Get("user_id"))
	if err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{"user_id": {"required, must be a UUID (see GET /dev/users)"}}))
		return
	}
	complexID, err := uuid.Parse(q.Get("complex_id"))
	if err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{"complex_id": {"required, must be a UUID (see GET /dev/users)"}}))
		return
	}
	wantRole := strings.ToUpper(q.Get("role"))

	users, err := h.load(r.Context(), &userID)
	if err != nil {
		errors.RespondError(w, nil, errors.NewInternalError(err))
		return
	}
	if len(users) == 0 {
		errors.RespondError(w, nil, errors.NewNotFoundError("User", userID.String()))
		return
	}
	user := users[0]

	var chosen *Membership
	for i := range user.Memberships {
		m := &user.Memberships[i]
		if m.ComplexID != complexID || (wantRole != "" && m.Role != wantRole) {
			continue
		}
		if chosen == nil || rolePriority[m.Role] > rolePriority[chosen.Role] {
			chosen = m
		}
	}
	if chosen == nil {
		errors.RespondError(w, nil, errors.NewPermissionError(wantRole))
		return
	}

	token, err := authmw.GenerateJWT(user.ID.String(), user.ID.String()+"@demo.local", chosen.Role, complexID.String())
	if err != nil {
		errors.RespondError(w, nil, errors.NewInternalError(err))
		return
	}
	errors.RespondOK(w, tokenResponse{
		Token: token, UserID: user.ID, Name: user.Name,
		ComplexID: chosen.ComplexID, ComplexName: chosen.ComplexName, Role: chosen.Role,
		UnitID: chosen.UnitID, UnitName: chosen.UnitName, Memberships: user.Memberships,
	})
}

// QRSystem handles GET /api/v1/dev/qr-system: simulated QR system + breaker state.
func (h *Handler) QRSystem(w http.ResponseWriter, r *http.Request) {
	errors.RespondOK(w, h.qr.Detail())
}

// SetQRSystem handles PUT /api/v1/dev/qr-system {"mode":"UP"|"DOWN"}.
// DOWN makes the simulated QR system hang (it never answers), which is what
// the call timeout and the circuit breaker are there to contain. The breaker is
// NOT reset: it recovers by itself after its cool-down, like in production.
func (h *Handler) SetQRSystem(w http.ResponseWriter, r *http.Request) {
	sim, ok := h.qr.Simulator()
	if !ok {
		errors.RespondError(w, nil, errors.NewBadRequestError("the QR system is not simulated in this environment"))
		return
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if err := errors.DecodeJSONLenient(w, r, &req); err != nil {
		errors.RespondError(w, nil, err)
		return
	}
	mode := strings.ToUpper(req.Mode)
	if mode != qrsystem.ModeUp && mode != qrsystem.ModeDown {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{"mode": {"must be UP or DOWN"}}))
		return
	}
	sim.SetMode(mode)
	errors.RespondOK(w, h.qr.Detail())
}

// ResetQRBreaker handles POST /api/v1/dev/qr-system/reset: closes the circuit now.
func (h *Handler) ResetQRBreaker(w http.ResponseWriter, r *http.Request) {
	h.qr.ResetBreaker()
	errors.RespondOK(w, h.qr.Detail())
}
