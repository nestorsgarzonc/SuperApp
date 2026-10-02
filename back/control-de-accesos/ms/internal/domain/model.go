package domain

import (
	"errors"
	"strings"
	"time"
)

const (
	RoleResident = "resident"
	RoleGuard    = "guard"

	StatusActive     = "active"
	StatusUsed       = "used"
	StatusExpired    = "expired"
	StatusRevoked    = "revoked"
	StatusCheckedOut = "checked_out"

	MaxValidity = 30 * 24 * time.Hour
)

var (
	ErrUnauthorized        = errors.New("invalid user")
	ErrForbidden           = errors.New("this action is not allowed for your role")
	ErrNotFound            = errors.New("visitor pass not found")
	ErrInvalidToken        = errors.New("invalid or unknown pass token")
	ErrUnitNotInComplex    = errors.New("unit does not belong to this complex")
	ErrUnitAccessDenied    = errors.New("you do not have access to this unit")
	ErrPassNotActive       = errors.New("pass is no longer active")
	ErrPassExpired         = errors.New("pass has expired")
	ErrPassNotYetValid     = errors.New("pass is not valid yet")
	ErrPassNotCheckedIn    = errors.New("pass is not checked in")
	ErrPassNotExtendable   = errors.New("only an active, unexpired pass can be extended")
	ErrInvalidExtension    = errors.New("new expiry must be later and within the maximum validity window")
	ErrVisitorNameRequired = errors.New("visitor name is required")
	ErrInvalidWindow       = errors.New("invalid validity window")
	ErrInvalidPlate        = errors.New("plate is required")
	ErrInvalidDirection    = errors.New("direction must be in or out")
	ErrPreauthNotFound     = errors.New("visitor pre-authorization not found")
)

type Actor struct {
	ComplexID string
	UserID    string
	FullName  string
	Role      string
	UnitIDs   []string
}

type Pass struct {
	ID           string     `json:"id"`
	ComplexID    string     `json:"complex_id"`
	UnitID       string     `json:"unit_id"`
	HostUserID   string     `json:"host_user_id"`
	VisitorName  string     `json:"visitor_name"`
	Document     string     `json:"document,omitempty"`
	Token        string     `json:"token,omitempty"`
	ValidFrom    time.Time  `json:"valid_from"`
	ValidUntil   time.Time  `json:"valid_until"`
	Status       string     `json:"status"`
	UsedBy       *string    `json:"used_by,omitempty"`
	UsedAt       *time.Time `json:"used_at,omitempty"`
	CheckedOutBy *string    `json:"checked_out_by,omitempty"`
	CheckedOutAt *time.Time `json:"checked_out_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (p *Pass) EffectiveStatus(now time.Time) string {
	if p.Status == StatusActive && !now.Before(p.ValidUntil) {
		return StatusExpired
	}
	return p.Status
}

func (p Pass) WithoutToken() Pass {
	p.Token = ""
	return p
}

type CreatePass struct {
	UnitID, VisitorName, Document string
	ValidFrom, ValidUntil         time.Time
}

type CheckpointActivity struct {
	PassID     string    `json:"pass_id"`
	Direction  string    `json:"direction"`
	UnitLabel  string    `json:"unit_label"`
	OccurredAt time.Time `json:"occurred_at"`
}

type CheckpointFeed struct {
	Activities      []CheckpointActivity `json:"activities"`
	EntriesLast24h  int                  `json:"entries_last_24h"`
	ExitsLast24h    int                  `json:"exits_last_24h"`
	CurrentlyInside int                  `json:"currently_inside"`
	GeneratedAt     time.Time            `json:"generated_at"`
}

type Preauth struct {
	ID          string    `json:"id"`
	ComplexID   string    `json:"complex_id"`
	UnitID      string    `json:"unit_id"`
	HostUserID  string    `json:"host_user_id"`
	Plate       string    `json:"plate"`
	VisitorName string    `json:"visitor_name"`
	ValidFrom   time.Time `json:"valid_from"`
	ValidUntil  time.Time `json:"valid_until"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type PreauthMatch struct {
	ID          string    `json:"id,omitempty"`
	VisitorName string    `json:"visitor_name"`
	HostName    string    `json:"host_name"`
	UnitNumber  string    `json:"unit_number,omitempty"`
	ValidUntil  time.Time `json:"valid_until,omitempty"`
}

type CreatePreauth struct {
	UnitID, Plate, VisitorName string
	ValidFrom, ValidUntil      time.Time
}

type AccessLog struct {
	ID             string        `json:"id"`
	ComplexID      string        `json:"complex_id"`
	Plate          string        `json:"plate"`
	PreauthID      *string       `json:"preauth_id,omitempty"`
	Direction      string        `json:"direction"`
	GuardID        string        `json:"guard_id"`
	Note           string        `json:"note,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	VisitorPreauth *PreauthMatch `json:"visitor_preauth,omitempty"`
}

type Lookup struct {
	Plate          string        `json:"plate"`
	Found          bool          `json:"found"`
	VisitorPreauth *PreauthMatch `json:"visitor_preauth,omitempty"`
}

type Complex struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Address  *string `json:"address"`
	Timezone string  `json:"timezone"`
}

type Person struct {
	ID          string  `json:"id"`
	FullName    string  `json:"full_name"`
	Email       *string `json:"email"`
	Role        string  `json:"role"`
	UnitID      *string `json:"unit_id"`
	UnitName    *string `json:"unit_name"`
	SectionName *string `json:"section_name"`
}

type Notification struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

func NormalizePlate(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(raw)) {
		switch r {
		case ' ', '-', '_', '\t', '.':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func NormalizeDirection(raw string) (string, error) {
	d := strings.ToLower(strings.TrimSpace(raw))
	if d == "in" || d == "out" {
		return d, nil
	}
	return "", ErrInvalidDirection
}
