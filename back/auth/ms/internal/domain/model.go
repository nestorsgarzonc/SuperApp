package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrAccountDisabled    = errors.New("account is not active")
	ErrEmailTaken         = errors.New("email is already registered")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrForbidden          = errors.New("permission denied")
	ErrNotFound           = errors.New("resource not found")
	ErrConflict           = errors.New("resource conflict")
	ErrInvalidArgument    = errors.New("invalid argument")
)

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type Role struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Scope       string  `json:"scope"`
	TenantID    *string `json:"tenant_id,omitempty"`
	Enabled     bool    `json:"enabled"`
}

type Permission struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Description string `json:"description"`
}

type Session struct {
	ID        string    `json:"id"`
	Device    string    `json:"device_name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

var (
	roleCodePattern       = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)
	permissionCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)+$`)
)

func ValidRoleCode(code string) bool       { return roleCodePattern.MatchString(code) }
func ValidPermissionCode(code string) bool { return permissionCodePattern.MatchString(code) }
