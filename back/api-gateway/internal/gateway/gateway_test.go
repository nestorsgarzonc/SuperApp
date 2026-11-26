package gateway

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"superapp/api-gateway/internal/authn"
	"superapp/api-gateway/internal/config"
	"superapp/api-gateway/internal/membership"
	"superapp/api-gateway/internal/ratelimit"
)

const (
	issuer      = "http://auth:8081"
	audience    = "superapp-api"
	kid         = "test-key"
	resident    = "10000000-0000-0000-0000-000000000001"
	otherUser   = "10000000-0000-0000-0000-000000000002"
	complexOne  = "00000000-0000-0000-0000-000000000001"
	complexTwo  = "00000000-0000-0000-0000-000000000002"
	devSecret   = "dev-hs256-secret-for-tests"
	tenantKey   = "0123456789abcdef0123456789abcdef"
	caSecret    = "ca-gateway-secret"
	notifSecret = "notif-gateway-secret"
)

// upstream records what a fake microservice received.
type upstream struct {
	*httptest.Server
	mu   sync.Mutex
	last *http.Request
	hits int
}

func newUpstream(t *testing.T) *upstream {
	u := &upstream{}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		u.last, u.hits = r.Clone(r.Context()), u.hits+1
		u.mu.Unlock()
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Access-Control-Allow-Origin", "*") // must be stripped by the gateway
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
	}))
	t.Cleanup(u.Close)
	return u
}

func (u *upstream) got() (*http.Request, int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.last, u.hits
}

type env struct {
	t      *testing.T
	key    *rsa.PrivateKey
	gw     *Gateway
	srv    *httptest.Server
	auth   *upstream
	ca     *upstream
	notif  *upstream
	caDown atomic.Bool
	revoke *fakeRevocations
}

type fakeRevocations struct {
	mu      sync.Mutex
	revoked map[string]bool
	err     error
}

func (f *fakeRevocations) Revoked(_ context.Context, sid string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.revoked[sid], f.err
}

func setup(t *testing.T, mutate ...func(*config.Config)) *env {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, key: key, auth: newUpstream(t), ca: newUpstream(t), notif: newUpstream(t), revoke: &fakeRevocations{revoked: map[string]bool{}}}

	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": kid,
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(jwks.Close)

	// Memberships: resident is RESIDENT of complexOne only.
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if e.caDown.Load() {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		if r.Header.Get("X-Internal-Secret") != "internal" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/internal/users/"+resident+"/complexes/"+complexOne+"/role" {
			_, _ = w.Write([]byte(`{"success":true,"data":{"role":"RESIDENT"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(internal.Close)

	cfg := config.Config{
		Env:                "development",
		Auth:               config.Upstream{URL: mustURL(e.auth.URL)},
		ControlDeAccesos:   config.Upstream{URL: mustURL(e.ca.URL), Secret: caSecret},
		Notificaciones:     config.Upstream{URL: mustURL(e.notif.URL), Secret: notifSecret},
		AuthJWKSURL:        jwks.URL,
		AuthIssuer:         issuer,
		AuthAudience:       audience,
		AuthTenantSecret:   tenantKey,
		InternalSecret:     "internal",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
		DevHS256Secret:     devSecret,
		ExposeDevRoutes:    true,
		PublicRatePerMin:   1000,
		UserRatePerMin:     1000,
		MaxBodyBytes:       1 << 10,
		UpstreamTimeout:    5 * time.Second,
		RoleCacheTTL:       time.Minute,
	}
	for _, m := range mutate {
		m(&cfg)
	}
	verifier := authn.NewVerifier(authn.Config{JWKSURL: cfg.AuthJWKSURL, Issuer: issuer, Audience: audience, DevHS256Secret: cfg.DevHS256Secret})
	// The real resolver, pointed at the fake internal endpoint. The cache is
	// disabled (1ns TTL) so "control-de-accesos down" is observable at once.
	roles := membership.NewResolver(mustURL(internal.URL), "internal", time.Nanosecond, nil)
	e.gw = New(cfg, verifier, roles, e.revoke, ratelimit.NewMemory(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.srv = httptest.NewServer(e.gw)
	t.Cleanup(e.srv.Close)
	return e
}

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

func (e *env) token(sub string, exp time.Time) string { return e.tokenWithSID(sub, "s1", exp) }

func (e *env) tokenWithSID(sub, sid string, exp time.Time) string {
	claims := jwt.MapClaims{
		"iss": issuer, "aud": []string{audience}, "sub": sub,
		"iat": time.Now().Unix(), "exp": exp.Unix(),
	}
	if sid != "" {
		claims["sid"] = sid
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	raw, err := tok.SignedString(e.key)
	if err != nil {
		e.t.Fatal(err)
	}
	return raw
}

func (e *env) do(method, path string, headers map[string]string, body string) *http.Response {
	e.t.Helper()
	req, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func errorCode(t *testing.T, resp *http.Response) string {
	t.Helper()
	var body struct {
		Success bool
		Error   struct{ Code string }
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Success {
		t.Fatal("error response with success=true")
	}
	return body.Error.Code
}

func bearer(tok string) map[string]string { return map[string]string{"Authorization": "Bearer " + tok} }

func with(h map[string]string, kv ...string) map[string]string {
	for i := 0; i+1 < len(kv); i += 2 {
		h[kv[i]] = kv[i+1]
	}
	return h
}

// Acceptance: a spoofed X-User-ID is replaced by the token's subject.
func TestAC01_SpoofedUserIDIsReplaced(t *testing.T) {
	e := setup(t)
	resp := e.do("GET", "/api/v1/access/qr-codes", with(bearer(e.token(resident, time.Now().Add(time.Hour))),
		"X-User-ID", otherUser, "X-Gateway-Secret", "forged", "X-Internal-Secret", "forged"), "")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	got, _ := e.ca.got()
	if got.Header.Get("X-User-ID") != resident {
		t.Fatalf("X-User-ID = %q", got.Header.Get("X-User-ID"))
	}
	if got.Header.Get("X-Gateway-Secret") != caSecret {
		t.Fatalf("X-Gateway-Secret = %q", got.Header.Get("X-Gateway-Secret"))
	}
	if got.Header.Get("X-Internal-Secret") != "" {
		t.Fatal("client X-Internal-Secret reached the service")
	}
	if got.URL.Path != "/api/v1/access/qr-codes" {
		t.Fatalf("path %q", got.URL.Path)
	}
}

// Acceptance: the role comes from the memberships, never from the client.
func TestAC02_ComplexRoleComesFromMemberships(t *testing.T) {
	e := setup(t)
	e.do("GET", "/api/v1/access/qr-codes", with(bearer(e.token(resident, time.Now().Add(time.Hour))),
		"X-Complex-ID", complexOne, "X-Complex-Role", "ADMIN"), "")
	got, hits := e.ca.got()
	if hits != 1 || got.Header.Get("X-Complex-Role") != "RESIDENT" || got.Header.Get("X-Complex-ID") != complexOne {
		t.Fatalf("hits=%d role=%q complex=%q", hits, got.Header.Get("X-Complex-Role"), got.Header.Get("X-Complex-ID"))
	}
}

// Acceptance: a complex the user does not belong to never reaches the service.
func TestAC03_ForeignComplexIs403(t *testing.T) {
	e := setup(t)
	resp := e.do("GET", "/api/v1/access/qr-codes", with(bearer(e.token(resident, time.Now().Add(time.Hour))), "X-Complex-ID", complexTwo), "")
	if resp.StatusCode != 403 || errorCode(t, resp) != "PERM_003" {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if _, hits := e.ca.got(); hits != 0 {
		t.Fatal("service was called")
	}
}

func TestAC04_MalformedComplexIs422(t *testing.T) {
	e := setup(t)
	resp := e.do("GET", "/api/v1/access/qr-codes", with(bearer(e.token(resident, time.Now().Add(time.Hour))), "X-Complex-ID", "nope"), "")
	if resp.StatusCode != 422 || errorCode(t, resp) != "VAL_004" {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

// Acceptance: expired, missing, forged or wrong-issuer tokens are 401.
func TestAC05_BadTokensAre401(t *testing.T) {
	e := setup(t)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	forged := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": issuer, "aud": audience, "sub": resident, "exp": time.Now().Add(time.Hour).Unix()})
	forged.Header["kid"] = kid
	forgedRaw, _ := forged.SignedString(other)
	wrongIss := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": "evil", "aud": audience, "sub": resident, "exp": time.Now().Add(time.Hour).Unix()})
	wrongIss.Header["kid"] = kid
	wrongIssRaw, _ := wrongIss.SignedString(e.key)
	none := "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"`+resident+`"}`)) + "."

	cases := map[string]map[string]string{
		"missing":    {},
		"expired":    bearer(e.token(resident, time.Now().Add(-time.Minute))),
		"forged":     bearer(forgedRaw),
		"wrong iss":  bearer(wrongIssRaw),
		"alg none":   bearer(none),
		"not bearer": {"Authorization": "Basic abc"},
	}
	for name, h := range cases {
		resp := e.do("GET", "/api/v1/notifications/users/me/notifications", h, "")
		if resp.StatusCode != 401 || errorCode(t, resp) != "AUTH_001" {
			t.Errorf("%s: status %d", name, resp.StatusCode)
		}
	}
	if _, hits := e.notif.got(); hits != 0 {
		t.Fatal("service was called")
	}
}

// Acceptance: /internal is never routed, including through the
// notificaciones prefix strip and encoded/dot-segment tricks.
func TestAC06_InternalAndTraversalAre404(t *testing.T) {
	e := setup(t)
	auth := bearer(e.token(resident, time.Now().Add(time.Hour)))
	for _, p := range []string{
		"/api/v1/internal/users",
		"/internal/users/" + resident + "/complexes/" + complexOne + "/role",
		"/api/v1/notifications/internal/notifications",
		"/api/v1/notifications/%69nternal/notifications",
		"/api/v1/notifications/x/%2e%2e/internal/notifications",
		"/api/v1/notifications/x%2f..%2finternal/notifications",
		"/api/v1/unknown",
		"/api/v1/accessories",
		"/.well-known/jwks.json",
	} {
		resp := e.do("POST", p, auth, "{}")
		if resp.StatusCode != 404 {
			t.Errorf("%s: status %d", p, resp.StatusCode)
		}
	}
	_, a := e.auth.got()
	_, c := e.ca.got()
	_, n := e.notif.got()
	if a+c+n != 0 {
		t.Fatalf("services were called: auth=%d ca=%d notif=%d", a, c, n)
	}
}

// Acceptance: control-de-accesos down → 503, never forwarded without a role.
func TestAC07_MembershipOutageFailsClosed(t *testing.T) {
	e := setup(t)
	e.caDown.Store(true)
	resp := e.do("GET", "/api/v1/notifications/users/me/notifications", with(bearer(e.token(resident, time.Now().Add(time.Hour))), "X-Complex-ID", complexOne), "")
	if resp.StatusCode != 503 || errorCode(t, resp) != "SYS_005" {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if _, hits := e.notif.got(); hits != 0 {
		t.Fatal("service was called")
	}
}

// Acceptance: exceeding the per-user limit is 429 with Retry-After.
func TestAC08_UserRateLimit(t *testing.T) {
	e := setup(t, func(c *config.Config) { c.UserRatePerMin = 3 })
	h := bearer(e.token(resident, time.Now().Add(time.Hour)))
	for i := 0; i < 3; i++ {
		if resp := e.do("GET", "/api/v1/units/x/access-grants", h, ""); resp.StatusCode != 200 {
			t.Fatalf("request %d: status %d", i, resp.StatusCode)
		}
	}
	resp := e.do("GET", "/api/v1/units/x/access-grants", h, "")
	if resp.StatusCode != 429 || resp.Header.Get("Retry-After") == "" || errorCode(t, resp) != "SYS_004" {
		t.Fatalf("status %d retry-after %q", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
}

func TestAC09_PublicAuthRoutes(t *testing.T) {
	e := setup(t, func(c *config.Config) { c.PublicRatePerMin = 2 })
	resp := e.do("POST", "/api/v1/auth/login", map[string]string{"X-User-ID": otherUser, "X-Complex-ID": complexOne}, `{"email":"a@b.c"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	got, _ := e.auth.got()
	if got.URL.Path != "/api/v1/auth/login" || got.Header.Get("X-User-ID") != "" || got.Header.Get("X-Gateway-Secret") != "" || got.Header.Get("X-Complex-ID") != "" {
		t.Fatalf("public request carried identity: path=%q headers=%v", got.URL.Path, got.Header)
	}
	e.do("POST", "/api/v1/auth/refresh", nil, "{}")
	if resp := e.do("POST", "/api/v1/auth/login", nil, "{}"); resp.StatusCode != 429 {
		t.Fatalf("per-IP limit not applied: %d", resp.StatusCode)
	}
	// Other auth routes still need a token.
	if resp := e.do("POST", "/api/v1/auth/logout-all", nil, "{}"); resp.StatusCode != 401 {
		t.Fatalf("logout-all without token: %d", resp.StatusCode)
	}
}

// Auth receives the validated complex as a signed tenant context.
func TestAC10_AuthGetsSignedTenant(t *testing.T) {
	e := setup(t)
	tok := e.token(resident, time.Now().Add(time.Hour))
	e.do("GET", "/api/v1/users/me", with(bearer(tok), "X-Complex-ID", complexOne, "X-Tenant-ID", complexTwo), "")
	got, _ := e.auth.got()
	ts := got.Header.Get(HeaderTenantTimestamp)
	if got.Header.Get(HeaderTenantID) != complexOne || got.Header.Get(HeaderTenantSignature) != SignTenant(tenantKey, resident, complexOne, ts) {
		t.Fatalf("tenant headers: %v", got.Header)
	}
	if got.Header.Get("Authorization") != "Bearer "+tok {
		t.Fatal("auth needs the bearer token to identify the caller")
	}
	if got.Header.Get("X-Gateway-Secret") != "" {
		t.Fatal("auth has no gateway secret configured; none should be sent")
	}
}

func TestAC11_NotificacionesPrefixIsStripped(t *testing.T) {
	e := setup(t)
	resp := e.do("PATCH", "/api/v1/notifications/notifications/abc/read?x=1", with(bearer(e.token(resident, time.Now().Add(time.Hour))), "X-Complex-ID", complexOne), "")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	got, _ := e.notif.got()
	if got.URL.Path != "/notifications/abc/read" || got.URL.RawQuery != "x=1" || got.Header.Get("X-Gateway-Secret") != notifSecret {
		t.Fatalf("path=%q query=%q secret=%q", got.URL.Path, got.URL.RawQuery, got.Header.Get("X-Gateway-Secret"))
	}
}

func TestAC12_BodyLimit(t *testing.T) {
	e := setup(t)
	resp := e.do("POST", "/api/v1/access-grants", bearer(e.token(resident, time.Now().Add(time.Hour))), strings.Repeat("x", 2<<10))
	if resp.StatusCode != 413 || errorCode(t, resp) != "SYS_003" {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestAC13_DevTokensOnlyWhenEnabled(t *testing.T) {
	dev := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"user_id": resident, "role": "ADMIN", "complex_id": complexTwo, "exp": time.Now().Add(time.Hour).Unix()})
	raw, _ := dev.SignedString([]byte(devSecret))

	e := setup(t)
	if resp := e.do("GET", "/api/v1/access/qr-codes", with(bearer(raw), "X-Complex-ID", complexOne), ""); resp.StatusCode != 200 {
		t.Fatalf("dev token rejected: %d", resp.StatusCode)
	}
	got, _ := e.ca.got()
	if got.Header.Get("X-Complex-Role") != "RESIDENT" {
		t.Fatalf("role claim of the dev token was trusted: %q", got.Header.Get("X-Complex-Role"))
	}
	if resp := e.do("GET", "/api/v1/dev/users", nil, ""); resp.StatusCode != 200 {
		t.Fatalf("dev users: %d", resp.StatusCode)
	}

	prod := setup(t, func(c *config.Config) { c.DevHS256Secret, c.ExposeDevRoutes = "", false })
	if resp := prod.do("GET", "/api/v1/access/qr-codes", bearer(raw), ""); resp.StatusCode != 401 {
		t.Fatalf("HS256 accepted without DEV_HS256_SECRET: %d", resp.StatusCode)
	}
	if resp := prod.do("GET", "/api/v1/dev/users", nil, ""); resp.StatusCode != 404 {
		t.Fatalf("dev routes exposed: %d", resp.StatusCode)
	}
}

func TestAC14_CORS(t *testing.T) {
	e := setup(t)
	resp := e.do("OPTIONS", "/api/v1/access/qr-codes", map[string]string{"Origin": "http://localhost:3000", "Access-Control-Request-Method": "POST"}, "")
	if resp.StatusCode != 204 || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:3000" || !strings.Contains(resp.Header.Get("Access-Control-Allow-Headers"), "X-Complex-ID") {
		t.Fatalf("preflight: %d %v", resp.StatusCode, resp.Header)
	}
	resp = e.do("OPTIONS", "/api/v1/access/qr-codes", map[string]string{"Origin": "https://evil.example", "Access-Control-Request-Method": "POST"}, "")
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("disallowed origin got CORS headers")
	}
	resp = e.do("GET", "/api/v1/access/qr-codes", with(bearer(e.token(resident, time.Now().Add(time.Hour))), "Origin", "http://localhost:3000"), "")
	if v := resp.Header.Values("Access-Control-Allow-Origin"); len(v) != 1 || v[0] != "http://localhost:3000" {
		t.Fatalf("upstream CORS header leaked or duplicated: %v", v)
	}
}

func TestAC15_UpstreamDownIs503(t *testing.T) {
	e := setup(t)
	e.notif.Close()
	resp := e.do("GET", "/api/v1/notifications/me/devices", bearer(e.token(resident, time.Now().Add(time.Hour))), "")
	if resp.StatusCode != 503 || errorCode(t, resp) != "SYS_005" {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestAC16_RequestIDPropagates(t *testing.T) {
	e := setup(t)
	resp := e.do("GET", "/api/v1/me/memberships", with(bearer(e.token(resident, time.Now().Add(time.Hour))), "X-Request-ID", "abc-123", "X-Correlation-ID", "spoof"), "")
	got, _ := e.ca.got()
	if resp.Header.Get("X-Request-ID") != "abc-123" || got.Header.Get("X-Request-ID") != "abc-123" || got.Header.Get("X-Correlation-ID") != "abc-123" {
		t.Fatalf("request id: resp=%q up=%q corr=%q", resp.Header.Get("X-Request-ID"), got.Header.Get("X-Request-ID"), got.Header.Get("X-Correlation-ID"))
	}
	resp = e.do("GET", "/healthz", with(map[string]string{}, "X-Request-ID", "bad id with spaces"), "")
	if id := resp.Header.Get("X-Request-ID"); id == "" || id == "bad id with spaces" {
		t.Fatalf("invalid request id kept: %q", id)
	}
}

// A session revoked by auth (logout) is rejected before its token expires.
func TestAC17_RevokedSessionIs401(t *testing.T) {
	e := setup(t)
	live := e.tokenWithSID(resident, "live", time.Now().Add(time.Hour))
	gone := e.tokenWithSID(resident, "gone", time.Now().Add(time.Hour))
	e.revoke.revoked["gone"] = true

	if resp := e.do("GET", "/api/v1/me/memberships", bearer(live), ""); resp.StatusCode != 200 {
		t.Fatalf("live session: %d", resp.StatusCode)
	}
	resp := e.do("GET", "/api/v1/me/memberships", bearer(gone), "")
	if resp.StatusCode != 401 || errorCode(t, resp) != "AUTH_001" {
		t.Fatalf("revoked session: %d", resp.StatusCode)
	}
	if resp := e.do("GET", "/api/v1/me/memberships", bearer(e.tokenWithSID(resident, "", time.Now().Add(time.Hour))), ""); resp.StatusCode != 401 {
		t.Fatalf("token without sid accepted: %d", resp.StatusCode)
	}
	// Redis down: degrade to "valid until exp" instead of locking everyone out.
	e.revoke.err = errors.New("redis down")
	if resp := e.do("GET", "/api/v1/me/memberships", bearer(live), ""); resp.StatusCode != 200 {
		t.Fatalf("revocation outage should fail open: %d", resp.StatusCode)
	}
}
