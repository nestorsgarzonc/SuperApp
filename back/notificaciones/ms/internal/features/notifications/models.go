package notifications

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	DeliveryStatusInboxOnly   = "INBOX_ONLY"
	DeliveryStatusPushSent    = "PUSH_SENT"
	DeliveryStatusPushPartial = "PUSH_PARTIAL"
	DeliveryStatusPushFailed  = "PUSH_FAILED"
)

const maxRecipients = 5000

type Notification struct {
	ID             uuid.UUID  `json:"id"`
	ComplexID      *uuid.UUID `json:"complex_id,omitempty"`
	UserID         uuid.UUID  `json:"user_id"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	Data           JSONB      `json:"data"`
	Type           string     `json:"type"`
	IsRead         bool       `json:"is_read"`
	ReadAt         *time.Time `json:"read_at,omitempty"`
	DeliveryStatus string     `json:"delivery_status"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type JSONB map[string]interface{}

func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = JSONB{}
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("failed to scan JSONB value: %v (type %T)", value, value)
	}
	m := map[string]interface{}{}
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	*j = m
	return nil
}

func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return []byte(`{}`), nil
	}
	b, err := json.Marshal(j)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

type DispatchInput struct {
	EventID   string                 `json:"event_id"`
	Source    string                 `json:"source"`
	ComplexID *uuid.UUID             `json:"complex_id"`
	UserIDs   []uuid.UUID            `json:"user_ids"`
	Category  string                 `json:"category"`
	Title     string                 `json:"title"`
	Body      string                 `json:"body"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

type NewNotifications struct {
	EventID   string
	Source    string
	ComplexID *uuid.UUID
	UserIDs   []uuid.UUID
	Title     string
	Body      string
	Data      JSONB
	Type      string
}
