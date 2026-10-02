package sessions

import (
	"time"

	"superapp/auth/internal/core/domain"
)

type SessionResponse struct {
	ID        string    `json:"id"`
	Device    string    `json:"device_name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func FromDomain(session domain.Session) SessionResponse {
	return SessionResponse{ID: session.ID, Device: session.Device, CreatedAt: session.CreatedAt, ExpiresAt: session.ExpiresAt}
}

func ListFromDomain(values []domain.Session) []SessionResponse {
	result := make([]SessionResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromDomain(value))
	}
	return result
}
