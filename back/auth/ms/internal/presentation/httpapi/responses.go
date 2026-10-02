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

func (a *API) respond(w http.ResponseWriter, err error, status int, payload any) {
	if err != nil {
		a.fail(w, err)
		return
	}
	if status == 204 {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, payload)
}

func (a *API) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, 403, "forbidden", "Permission denied")
	case errors.Is(err, domain.ErrInvalidCredentials), errors.Is(err, domain.ErrInvalidToken):
		writeError(w, 401, "unauthorized", "Authentication failed")
	case errors.Is(err, domain.ErrAccountDisabled):
		writeError(w, 403, "account_disabled", "Account is not active")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, 404, "not_found", "Resource not found")
	case errors.Is(err, domain.ErrEmailTaken), errors.Is(err, domain.ErrConflict):
		writeError(w, 409, "conflict", "Resource conflict")
	case errors.Is(err, domain.ErrInvalidArgument):
		writeError(w, 400, "invalid_request", err.Error())
	default:
		writeError(w, 500, "internal_error", "Internal server error")
	}
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, 400, "invalid_json", "Invalid request body")
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, 400, "invalid_json", "Request must contain one JSON value")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, 405, "method_not_allowed", "Method not allowed")
}

func validUUID(value string) bool { return uuidPattern.MatchString(value) }
