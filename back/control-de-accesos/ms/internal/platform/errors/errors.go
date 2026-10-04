package errors

import (
	"fmt"
	"net/http"
)

type ErrorCode string

type ErrorType int

const (
	TypeValidation ErrorType = iota
	TypeNotFound
	TypeAuth
	TypePermission
	TypeInternal
	TypeBadRequest
	TypeConflict
	TypeUnavailable
	TypePayloadTooLarge
)

type AppError interface {
	error
	Code() ErrorCode
	Type() ErrorType
	HTTPStatus() int
	Details() map[string]interface{}
	Unwrap() error
}

type baseError struct {
	code    ErrorCode
	errType ErrorType
	message string
	details map[string]interface{}
	cause   error
}

func (e *baseError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.message, e.cause)
	}
	return e.message
}

func (e *baseError) Code() ErrorCode                 { return e.code }
func (e *baseError) Type() ErrorType                 { return e.errType }
func (e *baseError) Details() map[string]interface{} { return e.details }
func (e *baseError) Unwrap() error                   { return e.cause }

func (e *baseError) HTTPStatus() int {
	switch e.errType {
	case TypeValidation:
		return http.StatusUnprocessableEntity
	case TypeNotFound:
		return http.StatusNotFound
	case TypeAuth:
		return http.StatusUnauthorized
	case TypePermission:
		return http.StatusForbidden
	case TypeBadRequest:
		return http.StatusBadRequest
	case TypeConflict:
		return http.StatusConflict
	case TypeUnavailable:
		return http.StatusServiceUnavailable
	case TypePayloadTooLarge:
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusInternalServerError
	}
}

type ValidationError struct {
	baseError
	FieldErrors map[string][]string
}

func NewValidationError(fieldErrors map[string][]string) *ValidationError {
	return &ValidationError{
		baseError: baseError{
			code:    ErrCodeValidationFailed,
			errType: TypeValidation,
			message: "Validation failed",
		},
		FieldErrors: fieldErrors,
	}
}

func (e *ValidationError) Details() map[string]interface{} {
	return map[string]interface{}{
		"field_errors": e.FieldErrors,
	}
}

type NotFoundError struct {
	baseError
	Resource string
	ID       string
}

func NewNotFoundError(resource, id string) *NotFoundError {
	return &NotFoundError{
		baseError: baseError{
			code:    ErrCodeResourceNotFound,
			errType: TypeNotFound,
			message: fmt.Sprintf("%s not found", resource),
		},
		Resource: resource,
		ID:       id,
	}
}

func (e *NotFoundError) Details() map[string]interface{} {
	return map[string]interface{}{
		"resource": e.Resource,
		"id":       e.ID,
	}
}

type AuthError struct {
	baseError
}

func NewAuthError(message string) *AuthError {
	return &AuthError{
		baseError: baseError{
			code:    ErrCodeAuthFailed,
			errType: TypeAuth,
			message: message,
		},
	}
}

type PermissionError struct {
	baseError
	RequiredRole string
}

func NewPermissionError(requiredRole string) *PermissionError {
	return &PermissionError{
		baseError: baseError{
			code:    ErrCodePermissionDenied,
			errType: TypePermission,
			message: "Permission denied",
		},
		RequiredRole: requiredRole,
	}
}

func (e *PermissionError) Details() map[string]interface{} {
	if e.RequiredRole != "" {
		return map[string]interface{}{
			"required_role": e.RequiredRole,
		}
	}
	return nil
}

type InternalError struct {
	baseError
}

func NewInternalError(cause error) *InternalError {
	return &InternalError{
		baseError: baseError{
			code:    ErrCodeInternalError,
			errType: TypeInternal,
			message: "An unexpected error occurred",
			cause:   cause,
		},
	}
}

type BadRequestError struct {
	baseError
}

func NewBadRequestError(message string) *BadRequestError {
	return &BadRequestError{
		baseError: baseError{
			code:    ErrCodeBadRequest,
			errType: TypeBadRequest,
			message: message,
		},
	}
}

// ConflictError: the request is well-formed but contradicts the current state
// (e.g. entering a building you're already inside).
type ConflictError struct {
	baseError
}

func NewConflictError(message string) *ConflictError {
	return &ConflictError{
		baseError: baseError{
			code:    ErrCodeConflict,
			errType: TypeConflict,
			message: message,
		},
	}
}

// UnavailableError: a dependency is down and the request was refused (or cut
// short) instead of left hanging. Details tell the client what to do instead.
type UnavailableError struct {
	baseError
}

func NewUnavailableError(code ErrorCode, message string, details map[string]interface{}) *UnavailableError {
	return &UnavailableError{
		baseError: baseError{
			code:    code,
			errType: TypeUnavailable,
			message: message,
			details: details,
		},
	}
}

// PayloadTooLargeError: the request body exceeds MaxRequestBodyBytes.
type PayloadTooLargeError struct {
	baseError
}

func NewPayloadTooLargeError(limit int64) *PayloadTooLargeError {
	return &PayloadTooLargeError{
		baseError: baseError{
			code:    ErrCodePayloadTooLarge,
			errType: TypePayloadTooLarge,
			message: "Request body too large",
			details: map[string]interface{}{"max_bytes": limit},
		},
	}
}

func Wrap(err error) AppError {
	if err == nil {
		return nil
	}
	if appErr, ok := err.(AppError); ok {
		return appErr
	}
	return NewInternalError(err)
}
