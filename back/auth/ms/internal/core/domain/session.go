package domain

import "time"

type Session struct {
	ID        string
	Device    string
	CreatedAt time.Time
	ExpiresAt time.Time
}
