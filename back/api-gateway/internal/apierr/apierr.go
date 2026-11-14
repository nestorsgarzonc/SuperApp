// Package apierr writes the gateway's own errors with the same envelope the
// microservices use, so the front parses a single error shape.
package apierr

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

type Error struct {
	Status  int
	Code    string
	Message string
}

var (
	Unauthenticated = Error{http.StatusUnauthorized, "AUTH_001", "Authentication required"}
	NotMember       = Error{http.StatusForbidden, "PERM_003", "You do not have access to this complex"}
	NotFound        = Error{http.StatusNotFound, "RES_001", "Route not found"}
	PayloadTooLarge = Error{http.StatusRequestEntityTooLarge, "SYS_003", "Request body too large"}
	BadComplexID    = Error{http.StatusUnprocessableEntity, "VAL_004", "X-Complex-ID must be a UUID"}
	RateLimited     = Error{http.StatusTooManyRequests, "SYS_004", "Too many requests"}
	Unavailable     = Error{http.StatusServiceUnavailable, "SYS_005", "Service temporarily unavailable"}
	UpstreamTimeout = Error{http.StatusGatewayTimeout, "SYS_005", "Service did not answer in time"}
)

type payload struct {
	Success bool `json:"success"`
	Error   struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func Write(w http.ResponseWriter, e Error) {
	var p payload
	p.Error.Code, p.Error.Message = e.Code, e.Message
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// WriteRetryAfter writes a 429 with Retry-After in whole seconds (at least 1).
func WriteRetryAfter(w http.ResponseWriter, retryAfter time.Duration) {
	secs := int(retryAfter.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	Write(w, RateLimited)
}
