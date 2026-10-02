package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"superapp/auth/internal/domain"
)

func (a *API) auth(w http.ResponseWriter, r *http.Request) (identity, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		writeError(w, 401, "unauthorized", "Bearer access token required")
		return identity{}, false
	}
	claims, err := a.service.Authenticate(strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		writeError(w, 401, "unauthorized", "Invalid or expired access token")
		return identity{}, false
	}
	return identity{claims.Subject, claims.Email, claims.SessionID}, true
}

func (a *API) requirePermission(w http.ResponseWriter, r *http.Request, permission string) (identity, bool) {
	id, ok := a.auth(w, r)
	if !ok {
		return identity{}, false
	}
	tenant, err := a.tenant(r, id.UserID)
	if err != nil {
		a.fail(w, err)
		return identity{}, false
	}
	allowed, err := a.service.HasPermission(r.Context(), id.UserID, permission, tenant)
	if err != nil {
		a.fail(w, err)
		return identity{}, false
	}
	if !allowed {
		writeError(w, 403, "forbidden", "Permission denied")
		return identity{}, false
	}
	return id, true
}

func (a *API) requireSystemPermission(w http.ResponseWriter, r *http.Request, permission string) (identity, bool) {
	id, ok := a.auth(w, r)
	if !ok {
		return identity{}, false
	}
	allowed, err := a.service.HasPermission(r.Context(), id.UserID, permission, nil)
	if err != nil {
		a.fail(w, err)
		return identity{}, false
	}
	if !allowed {
		writeError(w, 403, "forbidden", "Permission denied")
		return identity{}, false
	}
	return id, true
}

func (a *API) tenant(r *http.Request, userID string) (*string, error) {
	value := strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
	if value == "" {
		return nil, nil
	}
	if !validUUID(value) {
		return nil, domain.ErrForbidden
	}
	if len(a.tenantSecret) < 32 {
		return nil, domain.ErrForbidden
	}
	sig := r.Header.Get("X-Gateway-Signature")
	timestamp := r.Header.Get("X-Gateway-Timestamp")
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(unix, 0)) > time.Minute || time.Until(time.Unix(unix, 0)) > time.Minute {
		return nil, domain.ErrForbidden
	}
	mac := hmac.New(sha256.New, a.tenantSecret)
	_, _ = mac.Write([]byte(userID + "\n" + value + "\n" + timestamp))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.ToLower(sig)), []byte(expected)) {
		return nil, domain.ErrForbidden
	}
	return &value, nil
}

// tenantMemberAssertion validates a Gateway assertion that the target user, not
// merely the caller, belongs to the requested tenant.

func (a *API) tenantMemberAssertion(r *http.Request, actorID, targetID, tenantID string) error {
	timestamp := r.Header.Get("X-Gateway-Membership-Timestamp")
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(unix, 0)) > time.Minute || time.Until(time.Unix(unix, 0)) > time.Minute {
		return domain.ErrForbidden
	}
	mac := hmac.New(sha256.New, a.tenantSecret)
	_, _ = mac.Write([]byte(actorID + "\n" + targetID + "\n" + tenantID + "\n" + timestamp))
	expected := hex.EncodeToString(mac.Sum(nil))
	provided := strings.ToLower(r.Header.Get("X-Gateway-Membership-Signature"))
	if len(a.tenantSecret) < 32 || !hmac.Equal([]byte(provided), []byte(expected)) {
		return domain.ErrForbidden
	}
	return nil
}
