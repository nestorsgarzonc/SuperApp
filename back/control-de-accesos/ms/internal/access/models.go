package access

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	MethodQR      = "QR"
	MethodPreAuth = "PRE_AUTH"
)

var ErrAccessGrantNotFound = errors.New("access grant not found or expired")

// ErrUnitAccessDenied is returned when the caller is not associated with the
// grant's target unit. Complex scope alone is not enough: any complex member
// could otherwise read, mint or revoke a neighbor's grant.
var ErrUnitAccessDenied = errors.New("you do not have access to this unit")

var (
	ErrUnitNotFound        = errors.New("unit not found")
	ErrUnitComplexMismatch = errors.New("unit does not belong to the selected complex")
	ErrComplexIDRequired   = errors.New("complex ID is required")
	ErrQRCodeRequired      = errors.New("QR code is required for QR validation")
	ErrVisitorIDRequired   = errors.New("visitor ID number is required for PRE_AUTH validation")
	ErrUnsupportedMethod   = errors.New("unsupported validation method")
)

const (
	StatusActive    = "ACTIVE"
	StatusUsed      = "USED"
	StatusExpired   = "EXPIRED"
	StatusCancelled = "CANCELLED"
)

const (
	VisitorTypeDelivery   = "DELIVERY"
	VisitorTypeVisitor    = "VISITOR"
	VisitorTypeService    = "SERVICE"
	VisitorTypeContractor = "CONTRACTOR"
	VisitorTypeGuest      = "GUEST"
)

var validVisitorTypes = map[string]bool{
	VisitorTypeDelivery:   true,
	VisitorTypeVisitor:    true,
	VisitorTypeService:    true,
	VisitorTypeContractor: true,
	VisitorTypeGuest:      true,
}

type AccessGrant struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	ComplexID       uuid.UUID  `json:"complex_id" db:"complex_id"`
	UnitID          uuid.UUID  `json:"unit_id" db:"unit_id"`
	CreatedBy       uuid.UUID  `json:"created_by" db:"created_by"`
	Method          string     `json:"method" db:"method"`
	VisitorType     string     `json:"visitor_type" db:"visitor_type"`
	VisitorName     string     `json:"visitor_name" db:"visitor_name"`
	VisitorIDNumber *string    `json:"visitor_id_number,omitempty" db:"visitor_id_number"`
	QRCodeHash      *string    `json:"qr_code_hash,omitempty" db:"qr_code_hash"`
	Details         *string    `json:"details,omitempty" db:"details"`
	Status          string     `json:"status" db:"status"`
	ValidFrom       *time.Time `json:"valid_from,omitempty" db:"valid_from"`
	ValidUntil      *time.Time `json:"valid_until,omitempty" db:"valid_until"`
	UsedAt          *time.Time `json:"used_at,omitempty" db:"used_at"`
	UsedBy          *uuid.UUID `json:"used_by,omitempty" db:"used_by"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
}

type CreateAccessGrantRequest struct {
	UnitID          uuid.UUID  `json:"unit_id" validate:"required"`
	Method          string     `json:"method" validate:"required,oneof=QR PRE_AUTH"`
	VisitorType     string     `json:"visitor_type" validate:"required,oneof=DELIVERY VISITOR SERVICE CONTRACTOR GUEST"`
	VisitorName     string     `json:"visitor_name" validate:"required"`
	VisitorIDNumber *string    `json:"visitor_id_number,omitempty"`
	Details         *string    `json:"details,omitempty"`
	ValidFrom       *time.Time `json:"valid_from,omitempty"`
	ValidUntil      time.Time  `json:"valid_until" validate:"required"`
}

func (r CreateAccessGrantRequest) validate() map[string][]string {
	errs := map[string][]string{}
	if r.UnitID == uuid.Nil {
		errs["unit_id"] = []string{"required"}
	}
	if r.Method != MethodQR && r.Method != MethodPreAuth {
		errs["method"] = []string{"must be QR or PRE_AUTH"}
	}
	if !validVisitorTypes[r.VisitorType] {
		errs["visitor_type"] = []string{"must be one of DELIVERY, VISITOR, SERVICE, CONTRACTOR, GUEST"}
	}
	if r.VisitorName == "" {
		errs["visitor_name"] = []string{"required"}
	}
	if r.ValidUntil.IsZero() {
		errs["valid_until"] = []string{"required"}
	}
	return errs
}

type CreateQRCodeRequest struct {
	UnitID      uuid.UUID  `json:"unit_id" validate:"required"`
	VisitorName *string    `json:"visitor_name,omitempty"`
	ValidFrom   *time.Time `json:"valid_from,omitempty"`
	ValidUntil  time.Time  `json:"valid_until" validate:"required"`
}

func (r CreateQRCodeRequest) validate() map[string][]string {
	errs := map[string][]string{}
	if r.UnitID == uuid.Nil {
		errs["unit_id"] = []string{"required"}
	}
	if r.ValidUntil.IsZero() {
		errs["valid_until"] = []string{"required"}
	}
	return errs
}

type CreateQRBatchRequest struct {
	UnitID     uuid.UUID `json:"unit_id" validate:"required"`
	Quantity   int       `json:"quantity" validate:"required,min=1,max=100"`
	ValidFrom  time.Time `json:"valid_from" validate:"required"`
	ValidUntil time.Time `json:"valid_until" validate:"required"`
}

func (r CreateQRBatchRequest) validate() map[string][]string {
	errs := map[string][]string{}
	if r.UnitID == uuid.Nil {
		errs["unit_id"] = []string{"required"}
	}
	if r.Quantity < 1 || r.Quantity > 100 {
		errs["quantity"] = []string{"must be between 1 and 100"}
	}
	if r.ValidFrom.IsZero() {
		errs["valid_from"] = []string{"required"}
	}
	if r.ValidUntil.IsZero() {
		errs["valid_until"] = []string{"required"}
	}
	return errs
}

type CreatePreAuthRequest struct {
	UnitID          uuid.UUID `json:"unit_id" validate:"required"`
	VisitorName     string    `json:"visitor_name" validate:"required"`
	VisitorIDNumber string    `json:"visitor_id_number" validate:"required"`
	VisitorType     string    `json:"visitor_type" validate:"required"`
	ValidFrom       time.Time `json:"valid_from" validate:"required"`
	ValidUntil      time.Time `json:"valid_until" validate:"required"`
	Details         *string   `json:"details,omitempty"`
}

func (r CreatePreAuthRequest) validate() map[string][]string {
	errs := map[string][]string{}
	if r.UnitID == uuid.Nil {
		errs["unit_id"] = []string{"required"}
	}
	if r.VisitorName == "" {
		errs["visitor_name"] = []string{"required"}
	}
	if r.VisitorIDNumber == "" {
		errs["visitor_id_number"] = []string{"required"}
	}
	if !validVisitorTypes[r.VisitorType] {
		errs["visitor_type"] = []string{"must be one of DELIVERY, VISITOR, SERVICE, CONTRACTOR, GUEST"}
	}
	if r.ValidFrom.IsZero() {
		errs["valid_from"] = []string{"required"}
	}
	if r.ValidUntil.IsZero() {
		errs["valid_until"] = []string{"required"}
	}
	return errs
}

type AccessGrantResponse struct {
	AccessGrant
	CreatorName string `json:"creator_name"`
	UnitName    string `json:"unit_name"`
	QRCodeURL   string `json:"qr_code_url,omitempty"`
}

type ValidateAccessRequest struct {
	ComplexID       uuid.UUID `json:"complex_id" validate:"required"`
	Method          string    `json:"method" validate:"required"`
	QRCode          *string   `json:"qr_code,omitempty"`
	VisitorIDNumber *string   `json:"visitor_id_number,omitempty"`
}

type ValidateAccessResponse struct {
	Valid       bool         `json:"valid"`
	Message     string       `json:"message"`
	AccessGrant *AccessGrant `json:"access_grant,omitempty"`
	UnitID      *uuid.UUID   `json:"unit_id,omitempty"`
	VisitorName *string      `json:"visitor_name,omitempty"`
	VisitorType *string      `json:"visitor_type,omitempty"`
}
