package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"superapp/auth/internal/application"
	"superapp/auth/internal/domain"
)

type loginFailureService struct{ Service }

func (loginFailureService) Login(context.Context, string, string, string, string, string) (*domain.User, *application.TokenPair, error) {
	return nil, nil, domain.ErrInvalidCredentials
}

func TestInvalidLoginDoesNotRevealAddress(t *testing.T) {
	api := New(loginFailureService{}, nil)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"known@example.test","password":"wrong"}`))
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "known@example.test") {
		t.Fatalf("response leaked email: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Invalid email or password") {
		t.Fatalf("unexpected generic response: %s", w.Body.String())
	}
}

func TestTenantContextRequiresFreshGatewaySignature(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	api := New(loginFailureService{}, secret)
	now := time.Now().Unix()
	ts := strconv.FormatInt(now, 10)
	user, tenant := "00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000009"
	request := func(timestamp, signature string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Tenant-ID", tenant)
		r.Header.Set("X-Gateway-Timestamp", timestamp)
		r.Header.Set("X-Gateway-Signature", signature)
		return r
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(user + "\n" + tenant + "\n" + ts))
	valid := hex.EncodeToString(mac.Sum(nil))
	got, err := api.tenant(request(ts, valid), user)
	if err != nil || got == nil || *got != tenant {
		t.Fatalf("valid signature rejected: tenant=%v err=%v", got, err)
	}
	if _, err = api.tenant(request(ts, "00"), user); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("invalid signature accepted: %v", err)
	}
	old := strconv.FormatInt(now-120, 10)
	mac = hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(user + "\n" + tenant + "\n" + old))
	if _, err = api.tenant(request(old, hex.EncodeToString(mac.Sum(nil))), user); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("stale signature accepted: %v", err)
	}
}

func TestTenantContextWithoutHeaderDoesNotInventScope(t *testing.T) {
	api := New(loginFailureService{}, nil)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	tenant, err := api.tenant(r, "user")
	if err != nil || tenant != nil {
		t.Fatalf("unexpected tenant context: %v %v", tenant, err)
	}
}

func TestTenantMembershipAssertionBindsActorTargetAndTenant(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	api := New(loginFailureService{}, secret)
	actor := "00000000-0000-4000-8000-000000000001"
	target := "00000000-0000-4000-8000-000000000002"
	tenant := "00000000-0000-4000-8000-000000000009"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sign := func(a, u, t string) string {
		mac := hmac.New(sha256.New, secret)
		_, _ = mac.Write([]byte(a + "\n" + u + "\n" + t + "\n" + ts))
		return hex.EncodeToString(mac.Sum(nil))
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/users/"+target+"/roles", nil)
	r.Header.Set("X-Gateway-Membership-Timestamp", ts)
	r.Header.Set("X-Gateway-Membership-Signature", sign(actor, target, tenant))
	if err := api.tenantMemberAssertion(r, actor, target, tenant); err != nil {
		t.Fatalf("valid target membership assertion rejected: %v", err)
	}
	if err := api.tenantMemberAssertion(r, actor, actor, tenant); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("assertion for a different target accepted: %v", err)
	}
	r.Header.Set("X-Gateway-Membership-Signature", sign(actor, target, "00000000-0000-4000-8000-000000000010"))
	if err := api.tenantMemberAssertion(r, actor, target, tenant); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("assertion for a different tenant accepted: %v", err)
	}
}
