package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"superapp/api-gateway/internal/authn"
	"superapp/api-gateway/internal/config"
)

// ── dobles ───────────────────────────────────────────────────────────────────

const goodToken = "good-token"

// stubVerifier accepts goodToken (or what verify decides) and counts calls (stub + spy).
type stubVerifier struct {
	mu     sync.Mutex
	calls  int
	tokens []string
	id     authn.Identity
	err    error
}

func (s *stubVerifier) Verify(_ context.Context, raw string) (authn.Identity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.tokens = append(s.tokens, raw)
	if s.err != nil || raw != goodToken {
		return authn.Identity{}, authn.ErrInvalidToken
	}
	return s.id, nil
}

// spyRoles answers a fixed role / membership / error and records each lookup.
type spyRoles struct {
	mu     sync.Mutex
	calls  []string
	role   string
	member bool
	err    error
}

func (s *spyRoles) Role(_ context.Context, userID, complexID string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, userID+"|"+complexID)
	return s.role, s.member, s.err
}

func (s *spyRoles) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.calls) }

// spyRevocations answers revoked / err and counts calls.
type spyRevocations struct {
	mu      sync.Mutex
	calls   []string
	revoked bool
	err     error
}

func (s *spyRevocations) Revoked(_ context.Context, sid string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, sid)
	return s.revoked, s.err
}

// stubLimiter allows or denies every hit and records key and limit (stub + spy).
type stubLimiter struct {
	mu     sync.Mutex
	keys   []string
	limits []int
	deny   bool
	retry  time.Duration
	err    error
}

func (s *stubLimiter) Allow(_ context.Context, key string, limit int, _ time.Duration) (bool, time.Duration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys = append(s.keys, key)
	s.limits = append(s.limits, limit)
	if s.err != nil {
		return true, 0, s.err
	}
	return !s.deny, s.retry, nil
}

// logSpy is a slog.Handler that keeps every record (spy on the access log).
type logSpy struct {
	mu      sync.Mutex
	records []map[string]any
}

func (l *logSpy) Enabled(context.Context, slog.Level) bool { return true }
func (l *logSpy) WithAttrs([]slog.Attr) slog.Handler       { return l }
func (l *logSpy) WithGroup(string) slog.Handler            { return l }
func (l *logSpy) Handle(_ context.Context, r slog.Record) error {
	m := map[string]any{"msg": r.Message, "level": r.Level.String()}
	r.Attrs(func(a slog.Attr) bool { m[a.Key] = a.Value.Any(); return true })
	l.mu.Lock()
	l.records = append(l.records, m)
	l.mu.Unlock()
	return nil
}

func (l *logSpy) find(msg string) []map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []map[string]any
	for _, r := range l.records {
		if r["msg"] == msg {
			out = append(out, r)
		}
	}
	return out
}

// pipe is a gateway wired only to doubles; the three upstreams are fake
// microservices (httptest) that record what they receive.
type pipe struct {
	t        *testing.T
	gw       *Gateway
	srv      *httptest.Server
	verifier *stubVerifier
	roles    *spyRoles
	revoked  *spyRevocations
	limiter  *stubLimiter
	log      *logSpy
	auth     *upstream
	ca       *upstream
	notif    *upstream
}

var fixedNow = time.Unix(1_800_000_000, 0)

func newPipe(t *testing.T, mutate ...func(*config.Config)) *pipe {
	t.Helper()
	p := &pipe{
		t:        t,
		verifier: &stubVerifier{id: authn.Identity{UserID: resident, SessionID: "s1"}},
		roles:    &spyRoles{role: "RESIDENT", member: true},
		revoked:  &spyRevocations{},
		limiter:  &stubLimiter{},
		log:      &logSpy{},
		auth:     newUpstream(t),
		ca:       newUpstream(t),
		notif:    newUpstream(t),
	}
	cfg := config.Config{
		Env:                "production",
		Auth:               config.Upstream{URL: mustURL(p.auth.URL)},
		ControlDeAccesos:   config.Upstream{URL: mustURL(p.ca.URL), Secret: caSecret},
		Notificaciones:     config.Upstream{URL: mustURL(p.notif.URL), Secret: notifSecret},
		AuthTenantSecret:   tenantKey,
		CORSAllowedOrigins: []string{"http://localhost:3000"},
		PublicRatePerMin:   10,
		UserRatePerMin:     300,
		MaxBodyBytes:       100,
		UpstreamTimeout:    5 * time.Second,
	}
	for _, m := range mutate {
		m(&cfg)
	}
	p.gw = New(cfg, p.verifier, p.roles, p.revoked, p.limiter, slog.New(p.log))
	p.gw.now = func() time.Time { return fixedNow }
	p.srv = httptest.NewServer(p.gw)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *pipe) do(method, path string, headers map[string]string, body string) *http.Response {
	p.t.Helper()
	req, err := http.NewRequest(method, p.srv.URL+path, strings.NewReader(body))
	if err != nil {
		p.t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		p.t.Fatal(err)
	}
	p.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func (p *pipe) hits() int {
	_, a := p.auth.got()
	_, c := p.ca.got()
	_, n := p.notif.got()
	return a + c + n
}

func authed(kv ...string) map[string]string {
	return with(map[string]string{"Authorization": "Bearer " + goodToken}, kv...)
}

func expect(t *testing.T, resp *http.Response, status int, code string) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("status %d, esperaba %d", resp.StatusCode, status)
	}
	if code != "" {
		if got := errorCode(t, resp); got != code {
			t.Fatalf("code %q, esperaba %q", got, code)
		}
	}
}

// ── casos ────────────────────────────────────────────────────────────────────

// Caso PL-01 · cabeceras de identidad del cliente se descartan
// Técnica: Partición en clases de equivalencia (cada cabecera protegida, en varias cajas) y
// adivinación de errores (suplantación).
// Entrada: petición autenticada que trae cada cabecera de identidad o confianza falsificada
// Debe hacer / esperado: ninguna llega al servicio con el valor del cliente; las cabeceras comunes
// (Accept, X-Custom) sí llegan.
func TestPL01_CabecerasDeIdentidadSeDescartan(t *testing.T) {
	spoofed := []string{
		"X-User-ID", "x-user-id", "X-Complex-Role", "X-Tenant-ID", "X-Correlation-ID",
		"X-Gateway-Secret", "X-Gateway-Signature", "X-Gateway-Timestamp", "x-gateway-anything",
		"X-Internal-Secret", "X-Internal-Whatever", "X-Forwarded-Host", "X-Forwarded-Prefix",
		"Forwarded", "X-Real-Ip", "True-Client-Ip",
	}
	for _, h := range spoofed {
		t.Run("PL-01 "+h, func(t *testing.T) {
			p := newPipe(t)
			expect(t, p.do("GET", "/api/v1/access/qr-codes", authed(h, "forged", "Accept", "application/json", "X-Custom", "keep"), ""), 200, "")
			got, _ := p.ca.got()
			if got.Header.Get(h) == "forged" {
				t.Fatalf("%s llegó al servicio con el valor del cliente", h)
			}
			if got.Header.Get("X-Custom") != "keep" || got.Header.Get("Accept") != "application/json" {
				t.Fatal("se borró una cabecera común")
			}
		})
	}
}

// Caso PL-02 · token ausente o mal presentado
// Técnica: Tabla de decisión (forma del header Authorization).
// Entrada: sin header, esquema Basic, "Bearer" sin token, "Bearer" con solo espacios, token
// inválido
// Debe hacer / esperado: 401 AUTH_001; el verificador solo se llama cuando hay un token; nunca se
// consulta rol, revocación ni límite y el servicio no recibe nada.
func TestPL02_TokenAusenteOMalPresentado(t *testing.T) {
	cases := []struct {
		name    string
		header  string
		verifed bool
	}{
		{"PL-02a sin Authorization", "", false},
		{"PL-02b esquema Basic", "Basic abc", false},
		{"PL-02c Bearer sin token", "Bearer ", false},
		{"PL-02d Bearer con espacios", "Bearer    ", false},
		{"PL-02e Bearer sin espacio", "Bearer" + goodToken, false},
		{"PL-02f token inválido", "Bearer otro", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := newPipe(t)
			h := map[string]string{"X-Complex-ID": complexOne}
			if c.header != "" {
				h["Authorization"] = c.header
			}
			expect(t, p.do("GET", "/api/v1/access/qr-codes", h, ""), 401, "AUTH_001")
			if (p.verifier.calls == 1) != c.verifed || p.roles.count() != 0 || len(p.revoked.calls) != 0 || len(p.limiter.keys) != 0 || p.hits() != 0 {
				t.Fatalf("verify=%d roles=%d revoked=%d limiter=%d hits=%d", p.verifier.calls, p.roles.count(), len(p.revoked.calls), len(p.limiter.keys), p.hits())
			}
		})
	}
	t.Run("PL-02g espacios alrededor del token se recortan", func(t *testing.T) {
		p := newPipe(t)
		expect(t, p.do("GET", "/api/v1/access/qr-codes", map[string]string{"Authorization": "Bearer   " + goodToken + "  "}, ""), 200, "")
		if p.verifier.tokens[0] != goodToken {
			t.Fatalf("token verificado %q", p.verifier.tokens[0])
		}
	})
}

// Caso PL-03 · revocación de sesiones
// Técnica: Tabla de decisión (checker configurado x sid x respuesta de Redis).
// Entrada: sesión revocada, vigente, Redis con error, token dev sin sid, gateway sin Redis
// Debe hacer / esperado: revocada -> 401 sin consultar límite ni servicio; error -> se deja pasar
// y se registra un aviso; sin sid o sin checker no se consulta.
func TestPL03_RevocacionDeSesiones(t *testing.T) {
	t.Run("PL-03a revocada", func(t *testing.T) {
		p := newPipe(t)
		p.revoked.revoked = true
		expect(t, p.do("GET", "/api/v1/me/memberships", authed(), ""), 401, "AUTH_001")
		if p.revoked.calls[0] != "s1" || len(p.limiter.keys) != 0 || p.hits() != 0 {
			t.Fatalf("revoked=%v limiter=%d hits=%d", p.revoked.calls, len(p.limiter.keys), p.hits())
		}
	})
	t.Run("PL-03b vigente", func(t *testing.T) {
		p := newPipe(t)
		expect(t, p.do("GET", "/api/v1/me/memberships", authed(), ""), 200, "")
		if len(p.revoked.calls) != 1 {
			t.Fatalf("revoked=%v", p.revoked.calls)
		}
	})
	t.Run("PL-03c Redis con error deja pasar y avisa", func(t *testing.T) {
		p := newPipe(t)
		p.revoked.err = errors.New("redis down")
		expect(t, p.do("GET", "/api/v1/me/memberships", authed(), ""), 200, "")
		if len(p.log.find("revocation check unavailable, allowing token")) != 1 {
			t.Fatal("no se registró el aviso")
		}
	})
	t.Run("PL-03d token dev sin sid no se consulta", func(t *testing.T) {
		p := newPipe(t)
		p.verifier.id = authn.Identity{UserID: resident, DevToken: true}
		expect(t, p.do("GET", "/api/v1/me/memberships", authed(), ""), 200, "")
		if len(p.revoked.calls) != 0 {
			t.Fatalf("revoked=%v", p.revoked.calls)
		}
	})
	t.Run("PL-03e sin Redis no se consulta", func(t *testing.T) {
		p := newPipe(t)
		p.gw.revoked = nil
		expect(t, p.do("GET", "/api/v1/me/memberships", authed(), ""), 200, "")
	})
}

// Caso PL-04 · límite de peticiones
// Técnica: Tabla de decisión (ruta pública / autenticada x respuesta del limitador).
// Entrada: rutas públicas y autenticadas con el limitador permitiendo, negando o fallando
// Debe hacer / esperado: pública -> clave ip:<ip> con el límite público; autenticada -> clave
// user:<id> con el límite de usuario; negado -> 429 con Retry-After sin consultar rol ni llamar
// al servicio; error del limitador -> se deja pasar y se avisa.
func TestPL04_LimiteDePeticiones(t *testing.T) {
	t.Run("PL-04a clave y límite de ruta pública", func(t *testing.T) {
		p := newPipe(t)
		p.do("POST", "/api/v1/auth/login", nil, "{}")
		if len(p.limiter.keys) != 1 || p.limiter.keys[0] != "ip:127.0.0.1" || p.limiter.limits[0] != 10 {
			t.Fatalf("keys=%v limits=%v", p.limiter.keys, p.limiter.limits)
		}
	})
	t.Run("PL-04b clave y límite de usuario", func(t *testing.T) {
		p := newPipe(t)
		p.do("GET", "/api/v1/me/memberships", authed(), "")
		if len(p.limiter.keys) != 1 || p.limiter.keys[0] != "user:"+resident || p.limiter.limits[0] != 300 {
			t.Fatalf("keys=%v limits=%v", p.limiter.keys, p.limiter.limits)
		}
	})
	t.Run("PL-04c negado en ruta autenticada", func(t *testing.T) {
		p := newPipe(t)
		p.limiter.deny, p.limiter.retry = true, 42*time.Second
		resp := p.do("GET", "/api/v1/access/qr-codes", authed("X-Complex-ID", complexOne), "")
		if resp.Header.Get("Retry-After") != "42" {
			t.Fatalf("Retry-After %q", resp.Header.Get("Retry-After"))
		}
		expect(t, resp, 429, "SYS_004")
		if p.roles.count() != 0 || p.hits() != 0 {
			t.Fatalf("roles=%d hits=%d", p.roles.count(), p.hits())
		}
	})
	t.Run("PL-04d negado en ruta pública", func(t *testing.T) {
		p := newPipe(t)
		p.limiter.deny = true
		expect(t, p.do("POST", "/api/v1/auth/login", nil, "{}"), 429, "SYS_004")
		if p.hits() != 0 {
			t.Fatal("el servicio fue llamado")
		}
	})
	t.Run("PL-04e limitador con error deja pasar", func(t *testing.T) {
		p := newPipe(t)
		p.limiter.err = errors.New("redis down")
		expect(t, p.do("GET", "/api/v1/me/memberships", authed(), ""), 200, "")
		if len(p.log.find("rate limiter unavailable, allowing request")) != 1 {
			t.Fatal("no se registró el aviso")
		}
	})
	t.Run("PL-04f IP real detrás de proxy de confianza", func(t *testing.T) {
		p := newPipe(t, func(c *config.Config) { c.TrustForwardedFor = true })
		p.do("POST", "/api/v1/auth/login", map[string]string{"X-Forwarded-For": "6.6.6.6, 9.9.9.9"}, "{}")
		if p.limiter.keys[0] != "ip:9.9.9.9" {
			t.Fatalf("key %q", p.limiter.keys[0])
		}
	})
}

// Caso PL-05 · conjunto y rol
// Técnica: Tabla de decisión (X-Complex-ID x respuesta de membresías) y partición del formato
// del UUID.
// Entrada: sin conjunto, solo espacios, UUID en mayúsculas, con llaves, texto inválido; rol que
// falla o usuario que no es miembro
// Debe hacer / esperado: sin conjunto no se consulta rol ni se envía X-Complex-*; el UUID se
// normaliza antes de consultar y reenviar; formato inválido -> 422; error -> 503; no miembro -> 403.
func TestPL05_ConjuntoYRol(t *testing.T) {
	upper := strings.ToUpper(complexOne)
	t.Run("PL-05a sin conjunto", func(t *testing.T) {
		p := newPipe(t)
		expect(t, p.do("GET", "/api/v1/me/memberships", authed(), ""), 200, "")
		got, _ := p.ca.got()
		if p.roles.count() != 0 || got.Header.Get("X-Complex-ID") != "" || got.Header.Get("X-Complex-Role") != "" {
			t.Fatalf("roles=%d headers=%v", p.roles.count(), got.Header)
		}
	})
	t.Run("PL-05b conjunto con solo espacios", func(t *testing.T) {
		p := newPipe(t)
		expect(t, p.do("GET", "/api/v1/me/memberships", authed("X-Complex-ID", "   "), ""), 200, "")
		if p.roles.count() != 0 {
			t.Fatal("se consultó el rol")
		}
	})
	for name, raw := range map[string]string{"PL-05c mayúsculas": upper, "PL-05d con llaves": "{" + complexOne + "}", "PL-05e con espacios": "  " + complexOne + " "} {
		t.Run(name, func(t *testing.T) {
			p := newPipe(t)
			expect(t, p.do("GET", "/api/v1/access/qr-codes", authed("X-Complex-ID", raw), ""), 200, "")
			got, _ := p.ca.got()
			if p.roles.calls[0] != resident+"|"+complexOne || got.Header.Get("X-Complex-ID") != complexOne || got.Header.Get("X-Complex-Role") != "RESIDENT" {
				t.Fatalf("roles=%v headers=%v", p.roles.calls, got.Header)
			}
		})
	}
	for name, raw := range map[string]string{"PL-05f texto": "nope", "PL-05g UUID corto": complexOne[:35], "PL-05h inyección": complexOne + ",x"} {
		t.Run(name, func(t *testing.T) {
			p := newPipe(t)
			expect(t, p.do("GET", "/api/v1/access/qr-codes", authed("X-Complex-ID", raw), ""), 422, "VAL_004")
			if p.roles.count() != 0 || p.hits() != 0 {
				t.Fatal("siguió el pipeline")
			}
		})
	}
	t.Run("PL-05i membresías caídas", func(t *testing.T) {
		p := newPipe(t)
		p.roles.err = errors.New("down")
		expect(t, p.do("GET", "/api/v1/access/qr-codes", authed("X-Complex-ID", complexOne), ""), 503, "SYS_005")
		if p.hits() != 0 {
			t.Fatal("se reenvió sin rol")
		}
	})
	t.Run("PL-05j no es miembro", func(t *testing.T) {
		p := newPipe(t)
		p.roles.member = false
		expect(t, p.do("GET", "/api/v1/access/qr-codes", authed("X-Complex-ID", complexOne), ""), 403, "PERM_003")
		if p.hits() != 0 {
			t.Fatal("se reenvió")
		}
	})
}

// Caso PL-06 · identidad de confianza para cada servicio
// Técnica: Tabla de decisión (servicio x secreto configurado).
// Entrada: la misma petición autenticada a control-de-accesos, notificaciones y auth
// Debe hacer / esperado: cada servicio recibe X-User-ID y solo su propio X-Gateway-Secret; auth,
// que no tiene secreto, no recibe ninguno.
func TestPL06_IdentidadDeConfianzaPorServicio(t *testing.T) {
	p := newPipe(t)
	p.do("GET", "/api/v1/access/qr-codes", authed(), "")
	p.do("GET", "/api/v1/notifications/me/devices", authed(), "")
	p.do("GET", "/api/v1/users/me", authed(), "")
	for name, c := range map[string]struct {
		up     *upstream
		secret string
	}{"PL-06a control-de-accesos": {p.ca, caSecret}, "PL-06b notificaciones": {p.notif, notifSecret}, "PL-06c auth": {p.auth, ""}} {
		t.Run(name, func(t *testing.T) {
			got, hits := c.up.got()
			if hits != 1 || got.Header.Get("X-User-ID") != resident || got.Header.Get("X-Gateway-Secret") != c.secret {
				t.Fatalf("hits=%d user=%q secret=%q", hits, got.Header.Get("X-User-ID"), got.Header.Get("X-Gateway-Secret"))
			}
		})
	}
}

// Caso PL-07 · contexto de tenant firmado
// Técnica: Tabla de decisión (ruta de auth x conjunto x secreto HMAC).
// Entrada: peticiones con y sin conjunto, a auth y a control-de-accesos, con y sin
// AUTH_TENANT_HMAC_SECRET
// Debe hacer / esperado: solo auth + conjunto + secreto lleva X-Tenant-ID, el timestamp del reloj
// y la firma correcta; en cualquier otra combinación no se envían cabeceras de tenant.
func TestPL07_ContextoDeTenantFirmado(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		complex bool
		secret  string
		signed  bool
	}{
		{"PL-07a auth + conjunto + secreto", "/api/v1/users/me", true, tenantKey, true},
		{"PL-07b auth sin conjunto", "/api/v1/users/me", false, tenantKey, false},
		{"PL-07c auth sin secreto", "/api/v1/users/me", true, "", false},
		{"PL-07d control-de-accesos", "/api/v1/access/qr-codes", true, tenantKey, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := newPipe(t, func(cfg *config.Config) { cfg.AuthTenantSecret = c.secret })
			h := authed()
			if c.complex {
				h["X-Complex-ID"] = complexOne
			}
			expect(t, p.do("GET", c.path, h, ""), 200, "")
			up := p.auth
			if strings.HasPrefix(c.path, "/api/v1/access") {
				up = p.ca
			}
			got, _ := up.got()
			ts := "1800000000"
			if c.signed {
				if got.Header.Get(HeaderTenantID) != complexOne || got.Header.Get(HeaderTenantTimestamp) != ts ||
					got.Header.Get(HeaderTenantSignature) != SignTenant(tenantKey, resident, complexOne, ts) {
					t.Fatalf("tenant %v", got.Header)
				}
				return
			}
			for _, h := range []string{HeaderTenantID, HeaderTenantTimestamp, HeaderTenantSignature} {
				if got.Header.Get(h) != "" {
					t.Fatalf("%s enviado", h)
				}
			}
		})
	}
}

// Caso PL-08 · rutas públicas no llevan identidad
// Técnica: Partición y adivinación de errores.
// Entrada: POST /api/v1/auth/login con un token inválido, X-Complex-ID y X-User-ID
// Debe hacer / esperado: se reenvía sin verificar el token, sin X-User-ID, sin X-Complex-ID y sin
// secreto de gateway.
func TestPL08_RutasPublicasSinIdentidad(t *testing.T) {
	p := newPipe(t)
	expect(t, p.do("POST", "/api/v1/auth/login", map[string]string{"Authorization": "Bearer basura", "X-Complex-ID": complexOne, "X-User-ID": otherUser}, "{}"), 200, "")
	got, _ := p.auth.got()
	if p.verifier.calls != 0 || got.Header.Get("X-User-ID") != "" || got.Header.Get("X-Complex-ID") != "" || got.Header.Get("X-Gateway-Secret") != "" || got.Header.Get(HeaderTenantID) != "" {
		t.Fatalf("verify=%d headers=%v", p.verifier.calls, got.Header)
	}
}

// Caso PL-09 · tamaño del cuerpo
// Técnica: Valores límite (MAX_BODY_BYTES = 100: 100 y 101 bytes) con Content-Length y sin él
// (chunked).
// Entrada: POST autenticado con cuerpos de 100 y 101 bytes
// Debe hacer / esperado: 100 pasa; 101 -> 413 SYS_003, con Content-Length sin llamar al servicio.
func TestPL09_TamanoDelCuerpo(t *testing.T) {
	t.Run("PL-09a 100 bytes", func(t *testing.T) {
		p := newPipe(t)
		expect(t, p.do("POST", "/api/v1/access-grants", authed(), strings.Repeat("x", 100)), 200, "")
	})
	t.Run("PL-09b 101 bytes con Content-Length", func(t *testing.T) {
		p := newPipe(t)
		expect(t, p.do("POST", "/api/v1/access-grants", authed(), strings.Repeat("x", 101)), 413, "SYS_003")
		if p.hits() != 0 || p.verifier.calls != 0 {
			t.Fatalf("hits=%d verify=%d", p.hits(), p.verifier.calls)
		}
	})
	t.Run("PL-09c 101 bytes sin Content-Length", func(t *testing.T) {
		p := newPipe(t)
		req, _ := http.NewRequest("POST", p.srv.URL+"/api/v1/access-grants", io.MultiReader(strings.NewReader(strings.Repeat("x", 101))))
		req.Header.Set("Authorization", "Bearer "+goodToken)
		req.ContentLength = -1
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		expect(t, resp, 413, "SYS_003")
	})
}

// Caso PL-10 · fallas del servicio de destino
// Técnica: Adivinación de errores.
// Entrada: (a) el servicio tarda más que UPSTREAM_TIMEOUT; (b) el servicio está caído
// Debe hacer / esperado: (a) 504 SYS_005; (b) 503 SYS_005 y un aviso en el log con el request id.
func TestPL10_FallasDelServicioDeDestino(t *testing.T) {
	t.Run("PL-10a timeout", func(t *testing.T) {
		slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(2 * time.Second):
			case <-r.Context().Done():
			}
		}))
		t.Cleanup(slow.Close)
		p := newPipe(t, func(c *config.Config) {
			c.UpstreamTimeout = 100 * time.Millisecond
			c.ControlDeAccesos.URL = mustURL(slow.URL)
		})
		expect(t, p.do("GET", "/api/v1/access/qr-codes", authed(), ""), 504, "SYS_005")
	})
	t.Run("PL-10b servicio caído", func(t *testing.T) {
		p := newPipe(t)
		p.ca.Close()
		expect(t, p.do("GET", "/api/v1/access/qr-codes", authed("X-Request-ID", "req-1"), ""), 503, "SYS_005")
		warns := p.log.find("upstream unavailable")
		if len(warns) != 1 || warns[0]["request_id"] != "req-1" {
			t.Fatalf("avisos %v", warns)
		}
	})
}

// Caso PL-11 · respuesta del servicio
// Técnica: Partición.
// Entrada: el servicio responde con su propio X-Request-ID y Access-Control-Allow-Origin "*";
// la petición trae Origin y X-Forwarded-For falsos
// Debe hacer / esperado: el cliente ve el X-Request-ID del gateway y solo el CORS del gateway; el
// servicio no recibe Origin y recibe la IP real en X-Forwarded-For y X-Real-Ip.
func TestPL11_RespuestaDelServicio(t *testing.T) {
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "del-servicio")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("X-Seen-Origin", r.Header.Get("Origin"))
		w.Header().Set("X-Seen-XFF", r.Header.Get("X-Forwarded-For"))
		w.Header().Set("X-Seen-Real", r.Header.Get("X-Real-Ip"))
		w.Header().Set("X-Seen-Proto", r.Header.Get("X-Forwarded-Proto"))
	}))
	t.Cleanup(echo.Close)
	p := newPipe(t, func(c *config.Config) { c.ControlDeAccesos.URL = mustURL(echo.URL) })
	resp := p.do("GET", "/api/v1/access/qr-codes", authed("Origin", "http://localhost:3000", "X-Request-ID", "req-7", "X-Forwarded-For", "6.6.6.6"), "")
	h := resp.Header
	if h.Get("X-Request-ID") != "req-7" || len(h.Values("X-Request-ID")) != 1 {
		t.Fatalf("X-Request-ID %v", h.Values("X-Request-ID"))
	}
	if v := h.Values("Access-Control-Allow-Origin"); len(v) != 1 || v[0] != "http://localhost:3000" || h.Get("Access-Control-Allow-Credentials") != "" {
		t.Fatalf("CORS %v", h)
	}
	if h.Get("X-Seen-Origin") != "" || h.Get("X-Seen-XFF") != "127.0.0.1" || h.Get("X-Seen-Real") != "127.0.0.1" || h.Get("X-Seen-Proto") != "http" {
		t.Fatalf("visto por el servicio: origin=%q xff=%q real=%q proto=%q", h.Get("X-Seen-Origin"), h.Get("X-Seen-XFF"), h.Get("X-Seen-Real"), h.Get("X-Seen-Proto"))
	}
}

// Caso PL-12 · identificador de petición
// Técnica: Valores límite (1 y 128 caracteres válidos, 129) y partición de caracteres.
// Entrada: X-Request-ID del cliente de distintas formas
// Debe hacer / esperado: un id válido se conserva en la respuesta y llega al servicio como
// X-Request-ID y X-Correlation-ID; uno inválido o ausente se reemplaza por un UUID nuevo.
func TestPL12_IdentificadorDePeticion(t *testing.T) {
	cases := []struct {
		name, in string
		kept     bool
	}{
		{"PL-12a un carácter", "a", true},
		{"PL-12b 128 caracteres", strings.Repeat("a", 128), true},
		{"PL-12c caracteres permitidos", "A-z_0.9:x", true},
		{"PL-12d 129 caracteres", strings.Repeat("a", 129), false},
		{"PL-12e con espacio", "a b", false},
		{"PL-12f con barra", "a/b", false},
		{"PL-12g ausente", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := newPipe(t)
			h := authed()
			if c.in != "" {
				h["X-Request-ID"] = c.in
			}
			resp := p.do("GET", "/api/v1/me/memberships", h, "")
			got, _ := p.ca.got()
			id := resp.Header.Get("X-Request-ID")
			if c.kept != (id == c.in) || (!c.kept && len(id) != 36) {
				t.Fatalf("id %q", id)
			}
			if got.Header.Get("X-Request-ID") != id || got.Header.Get("X-Correlation-ID") != id {
				t.Fatalf("servicio recibió %q / %q", got.Header.Get("X-Request-ID"), got.Header.Get("X-Correlation-ID"))
			}
		})
	}
}

// Caso PL-13 · healthz
// Técnica: Partición (path exacto y variantes).
// Entrada: GET /healthz sin token y GET /healthz/
// Debe hacer / esperado: /healthz responde 200 {"status":"ok"} sin verificar nada ni llamar a
// servicios; /healthz/ no existe (404).
func TestPL13_Healthz(t *testing.T) {
	p := newPipe(t)
	resp := p.do("GET", "/healthz", nil, "")
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != `{"status":"ok"}` || p.verifier.calls != 0 || p.hits() != 0 {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	expect(t, p.do("GET", "/healthz/", nil, ""), 404, "RES_001")
}

// Caso PL-14 · el orden de los controles corta antes del servicio
// Técnica: Tabla de decisión (primer control que falla).
// Entrada: rutas desconocidas o internas con token, path traversal y cuerpo grande sin token
// Debe hacer / esperado: 404 o 413 antes de verificar el token (el verificador no se llama).
func TestPL14_OrdenDeLosControles(t *testing.T) {
	cases := []struct {
		name, path, body string
		status           int
	}{
		{"PL-14a ruta desconocida", "/api/v1/nope", "", 404},
		{"PL-14b ruta interna", "/api/v1/notifications/internal/x", "", 404},
		{"PL-14c traversal", "/api/v1/notifications/%2e%2e/internal", "", 404},
		{"PL-14d cuerpo grande sin token", "/api/v1/access-grants", strings.Repeat("x", 200), 413},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := newPipe(t)
			resp := p.do("POST", c.path, nil, c.body)
			if resp.StatusCode != c.status || p.verifier.calls != 0 || p.hits() != 0 {
				t.Fatalf("status %d verify=%d hits=%d", resp.StatusCode, p.verifier.calls, p.hits())
			}
		})
	}
}

// Caso PL-15 · registro de acceso
// Técnica: Partición (petición aceptada y rechazada).
// Entrada: una petición reenviada (200) y otra rechazada por no ser miembro (403)
// Debe hacer / esperado: una línea "request" por petición con método, path, ruta, status real,
// request id y usuario, sin el token.
func TestPL15_RegistroDeAcceso(t *testing.T) {
	p := newPipe(t)
	p.do("GET", "/api/v1/me/memberships", authed("X-Request-ID", "ok-1"), "")
	p.roles.member = false
	p.do("GET", "/api/v1/access/qr-codes", authed("X-Request-ID", "deny-1", "X-Complex-ID", complexOne), "")
	logs := p.log.find("request")
	if len(logs) != 2 {
		t.Fatalf("%d líneas", len(logs))
	}
	want := []struct {
		status int64
		id     string
	}{{200, "ok-1"}, {403, "deny-1"}}
	for i, l := range logs {
		if l["status"] != want[i].status || l["request_id"] != want[i].id || l["route"] != "control-de-accesos" || l["user_id"] != resident || l["method"] != "GET" {
			t.Fatalf("línea %d: %v", i, l)
		}
		for k, v := range l {
			if s, ok := v.(string); ok && strings.Contains(s, goodToken) {
				t.Fatalf("el token aparece en el log (%s)", k)
			}
		}
	}
}

// Caso PL-16 · respuestas en streaming
// Técnica: Adivinación de errores (buffer intermedio).
// Entrada: el servicio envía un evento SSE y se queda abierto
// Debe hacer / esperado: el cliente recibe el primer evento antes de que el servicio termine (el
// registro de status no impide el Flush del proxy).
func TestPL16_RespuestasEnStreaming(t *testing.T) {
	release := make(chan struct{})
	sse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: hola\n\n"))
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(sse.Close)
	t.Cleanup(func() { close(release) })
	p := newPipe(t, func(c *config.Config) { c.Notificaciones.URL = mustURL(sse.URL) })
	resp := p.do("GET", "/api/v1/notifications/stream", authed(), "")
	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 12)
		n, _ := io.ReadFull(resp.Body, buf)
		got <- string(buf[:n])
	}()
	select {
	case s := <-got:
		if s != "data: hola\n\n" {
			t.Fatalf("recibido %q", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("el evento quedó retenido en el gateway")
	}
}

// Caso PL-17 · el cliente se va antes de la respuesta
// Técnica: Adivinación de errores.
// Entrada: el servicio tarda; el cliente cancela la petición cuando el servicio ya la recibió
// Debe hacer / esperado: no se registra "upstream unavailable" (no es una caída del servicio) y
// la línea de acceso se escribe igual.
func TestPL17_ElClienteSeVa(t *testing.T) {
	received := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(received)
		<-r.Context().Done()
	}))
	t.Cleanup(slow.Close)
	p := newPipe(t, func(c *config.Config) { c.ControlDeAccesos.URL = mustURL(slow.URL) })

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", p.srv.URL+"/api/v1/access/qr-codes", nil)
	req.Header.Set("Authorization", "Bearer "+goodToken)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	}()
	<-received
	cancel()
	<-done
	waitLog(t, p.log, "request")
	if w := p.log.find("upstream unavailable"); len(w) != 0 {
		t.Fatalf("se registró como caída: %v", w)
	}
	// La línea de acceso registra status 200 aunque no hubo respuesta (hallazgo G3).
}

func waitLog(t *testing.T, l *logSpy, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for len(l.find(msg)) == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("no apareció %q en el log", msg)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Caso PL-18 · los errores del servicio llegan tal cual
// Técnica: Partición en clases de equivalencia (un status por clase: 4xx de negocio, 409, 5xx).
// Entrada: el servicio responde 400, 404, 409 y 500 con su propio sobre de error
// Debe hacer / esperado: el cliente recibe el mismo status, el mismo cuerpo y el mismo
// Content-Type; el gateway no lo convierte en uno de sus errores (502/503).
func TestPL18_ErroresDelServicioLleganTalCual(t *testing.T) {
	for _, status := range []int{400, 404, 409, 500} {
		t.Run(fmt.Sprintf("PL-18 status %d", status), func(t *testing.T) {
			body := fmt.Sprintf(`{"success":false,"error":{"code":"SVC_%d","message":"del servicio"}}`, status)
			svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(svc.Close)
			p := newPipe(t, func(c *config.Config) { c.ControlDeAccesos.URL = mustURL(svc.URL) })
			resp := p.do("POST", "/api/v1/access-grants", authed(), "{}")
			got, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != status || string(got) != body || resp.Header.Get("Content-Type") != "application/problem+json" {
				t.Fatalf("status %d content-type %q body %s", resp.StatusCode, resp.Header.Get("Content-Type"), got)
			}
			if len(p.log.find("upstream unavailable")) != 0 {
				t.Fatal("un error de negocio se registró como caída")
			}
		})
	}
}
