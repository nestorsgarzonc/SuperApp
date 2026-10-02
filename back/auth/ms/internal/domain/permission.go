package domain

import "regexp"

type Permission struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Description string `json:"description"`
}

var permissionCodePattern = regexp.MustCompile("^[a-z][a-z0-9_-]*(\\.[a-z][a-z0-9_-]*)+$")

func ValidPermissionCode(code string) bool { return permissionCodePattern.MatchString(code) }
