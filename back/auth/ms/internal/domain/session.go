package domain

import "time"

type Session struct {
	ID        string    `json:"id"`
	Device    string    `json:"device_name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
