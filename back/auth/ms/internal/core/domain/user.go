package domain

import (
	"strings"
	"time"
)

type User struct {
	ID        string
	Email     string
	FullName  string
	Status    string
	CreatedAt time.Time
}

func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }
