package domain

import (
	"strings"
	"time"
)

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }
