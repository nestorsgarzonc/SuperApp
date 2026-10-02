package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"

	"superapp/auth/internal/domain"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (a *API) Respond(w http.ResponseWriter, err error, status int, payload any) {
	if err != nil {
		a.Fail(w, err)
		return
	}
	if status == 204 {
		w.WriteHeader(status)
		return
	}
	WriteJSON(w, status, payload)
}

func (a *API) Fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrForbidden):
		WriteError(w, 403, "forbidden", "Permission denied")
	case errors.Is(err, domain.ErrInvalidCredentials), errors.Is(err, domain.ErrInvalidToken):
		WriteError(w, 401, "unauthorized", "Authentication failed")
	case errors.Is(err, domain.ErrAccountDisabled):
		WriteError(w, 403, "account_disabled", "Account is not active")
	case errors.Is(err, domain.ErrNotFound):
		WriteError(w, 404, "not_found", "Resource not found")
	case errors.Is(err, domain.ErrEmailTaken), errors.Is(err, domain.ErrConflict):
		WriteError(w, 409, "conflict", "Resource conflict")
	case errors.Is(err, domain.ErrInvalidArgument):
		WriteError(w, 400, "invalid_request", err.Error())
	default:
		WriteError(w, 500, "internal_error", "Internal server error")
	}
}

func Decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		WriteError(w, 400, "invalid_json", "Invalid request body")
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		WriteError(w, 400, "invalid_json", "Request must contain one JSON value")
		return false
	}
	return true
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func MethodNotAllowed(w http.ResponseWriter) {
	WriteError(w, 405, "method_not_allowed", "Method not allowed")
}

func ValidUUID(value string) bool { return uuidPattern.MatchString(value) }
