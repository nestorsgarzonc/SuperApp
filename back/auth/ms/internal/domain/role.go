package domain

import "regexp"

type Role struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Scope       string  `json:"scope"`
	TenantID    *string `json:"tenant_id,omitempty"`
	Enabled     bool    `json:"enabled"`
}

var roleCodePattern = regexp.MustCompile("^[A-Z][A-Z0-9_]{1,63}$")

func ValidRoleCode(code string) bool { return roleCodePattern.MatchString(code) }
