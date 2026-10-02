package domain

import "regexp"

type Role struct {
	ID          string
	Code        string
	Name        string
	Description string
	Scope       string
	TenantID    *string
	Enabled     bool
}

var roleCodePattern = regexp.MustCompile("^[A-Z][A-Z0-9_]{1,63}$")

func ValidRoleCode(code string) bool { return roleCodePattern.MatchString(code) }
