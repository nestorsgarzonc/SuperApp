package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"superapp/auth/internal/core/domain"
)

func (a *API) Auth(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		WriteError(w, 401, "unauthorized", "Bearer access token required")
		return Identity{}, false
	}
	claims, err := a.Authentication.Authenticate(strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		WriteError(w, 401, "unauthorized", "Invalid or expired access token")
		return Identity{}, false
	}
	return Identity{claims.Subject, claims.Email, claims.SessionID}, true
}

func (a *API) RequirePermission(w http.ResponseWriter, r *http.Request, permission string) (Identity, bool) {
	id, ok := a.Auth(w, r)
	if !ok {
		return Identity{}, false
	}
	tenant, err := a.Tenant(r, id.UserID)
	if err != nil {
		a.Fail(w, err)
		return Identity{}, false
	}
	allowed, err := a.Authorization.HasPermission(r.Context(), id.UserID, permission, tenant)
	if err != nil {
		a.Fail(w, err)
		return Identity{}, false
	}
	if !allowed {
		WriteError(w, 403, "forbidden", "Permission denied")
		return Identity{}, false
	}
	return id, true
}

func (a *API) RequireSystemPermission(w http.ResponseWriter, r *http.Request, permission string) (Identity, bool) {
	id, ok := a.Auth(w, r)
	if !ok {
		return Identity{}, false
	}
	allowed, err := a.Authorization.HasPermission(r.Context(), id.UserID, permission, nil)
	if err != nil {
		a.Fail(w, err)
		return Identity{}, false
	}
	if !allowed {
		WriteError(w, 403, "forbidden", "Permission denied")
		return Identity{}, false
	}
	return id, true
}

func (a *API) Tenant(r *http.Request, userID string) (*string, error) {
	value := strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
	if value == "" {
		return nil, nil
	}
	if !ValidUUID(value) {
		return nil, domain.ErrForbidden
	}
	if len(a.TenantSecret) < 32 {
		return nil, domain.ErrForbidden
	}
	sig := r.Header.Get("X-Gateway-Signature")
	timestamp := r.Header.Get("X-Gateway-Timestamp")
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(unix, 0)) > time.Minute || time.Until(time.Unix(unix, 0)) > time.Minute {
		return nil, domain.ErrForbidden
	}
	mac := hmac.New(sha256.New, a.TenantSecret)
	_, _ = mac.Write([]byte(userID + "\n" + value + "\n" + timestamp))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.ToLower(sig)), []byte(expected)) {
		return nil, domain.ErrForbidden
	}
	return &value, nil
}

// tenantMemberAssertion validates a Gateway assertion that the target user, not
// merely the caller, belongs to the requested tenant.

func (a *API) TenantMemberAssertion(r *http.Request, actorID, targetID, tenantID string) error {
	timestamp := r.Header.Get("X-Gateway-Membership-Timestamp")
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(unix, 0)) > time.Minute || time.Until(time.Unix(unix, 0)) > time.Minute {
		return domain.ErrForbidden
	}
	mac := hmac.New(sha256.New, a.TenantSecret)
	_, _ = mac.Write([]byte(actorID + "\n" + targetID + "\n" + tenantID + "\n" + timestamp))
	expected := hex.EncodeToString(mac.Sum(nil))
	provided := strings.ToLower(r.Header.Get("X-Gateway-Membership-Signature"))
	if len(a.TenantSecret) < 32 || !hmac.Equal([]byte(provided), []byte(expected)) {
		return domain.ErrForbidden
	}
	return nil
}
