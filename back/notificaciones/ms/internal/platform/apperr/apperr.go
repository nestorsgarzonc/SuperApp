package apperr

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

type Kind int

const (
	KindInternal Kind = iota
	KindValidation
	KindNotFound
	KindAuth
	KindUnavailable
)

type Error struct {
	Kind    Kind
	Code    string
	Message string
	Fields  map[string][]string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

func (e *Error) HTTPStatus() int {
	switch e.Kind {
	case KindValidation:
		return http.StatusUnprocessableEntity
	case KindNotFound:
		return http.StatusNotFound
	case KindAuth:
		return http.StatusUnauthorized
	case KindUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func Validation(fields map[string][]string) *Error {
	return &Error{Kind: KindValidation, Code: "VAL_001", Message: "Validation failed", Fields: fields}
}

func Field(field, message string) *Error {
	return Validation(map[string][]string{field: {message}})
}

func NotFound(resource, id string) *Error {
	return &Error{Kind: KindNotFound, Code: "RES_001", Message: fmt.Sprintf("%s %s not found", resource, id)}
}

func Auth(message string) *Error {
	return &Error{Kind: KindAuth, Code: "AUTH_001", Message: message}
}

func Unavailable(message string, cause error) *Error {
	return &Error{Kind: KindUnavailable, Code: "SYS_005", Message: message, Cause: cause}
}

func Internal(cause error) *Error {
	return &Error{Kind: KindInternal, Code: "SYS_001", Message: "An unexpected error occurred", Cause: cause}
}

func IsValidation(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Kind == KindValidation
}

type errorPayload struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func RespondError(w http.ResponseWriter, logger *slog.Logger, err error) {
	var appErr *Error
	if !errors.As(err, &appErr) {
		appErr = Internal(err)
	}
	if appErr.Kind == KindInternal && logger != nil {
		logger.Error("internal error", "error", err)
	}

	payload := errorPayload{Code: appErr.Code, Message: appErr.Message}
	if appErr.Kind != KindInternal && appErr.Cause == nil {
		payload.Message = appErr.Error()
	}
	if len(appErr.Fields) > 0 {
		payload.Details = map[string]interface{}{"fields": appErr.Fields}
	}
	writeJSON(w, appErr.HTTPStatus(), map[string]interface{}{"success": false, "error": payload})
}

func RespondOK(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": data})
}

func RespondCreated(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": data})
}

func RespondAccepted(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusAccepted, map[string]interface{}{"success": true, "data": data})
}

func RespondNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func DecodeJSON(r *http.Request, dst interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return Field("body", "Invalid JSON body")
	}
	return nil
}
