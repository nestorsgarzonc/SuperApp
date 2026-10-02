package domain

import "errors"

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
