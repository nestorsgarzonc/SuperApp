package middleware

// Pruebas unitarias de auth.go (componente C8) — casos MW-01…MW-14.
//
// Aislamiento (sin t.Parallel en este paquete: JWT_SECRET, GATEWAY_SECRET y ENV
// son estado global del proceso):
//   - entorno        -> fixture controlado con t.Setenv (no hay costura sin refactor).
//   - checkers       -> stubs con spy (stubComplexChecker, stubUnitChecker, stubStaffChecker).
//   - siguiente handler -> spy (spyNext) que captura el contexto recibido.
//   - respuesta HTTP -> httptest.NewRecorder (sin red ni sockets).
//   - reloj          -> tokens con expiración relativa (±60 s); nunca se prueba la igualdad
//     exacta exp==now porque auth.go lee time.Now() sin costura.
//   - RS256/attachComplex quedan fuera (ver MW-S1..S3, semiintegración).

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apperrors "controldeaccesos/internal/platform/errors"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const unitSecret = "unit-test-secret"

// ---------------------------------------------------------------------------
// Dobles
// ---------------------------------------------------------------------------

// spyNext es un SPY del handler siguiente: registra si se invocó y el contexto
// con el que se invocó.
type spyNext struct {
	calls int
	ctx   context.Context
}

func (s *spyNext) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	s.calls++
	s.ctx = r.Context()
}

// stubComplexChecker es un STUB (respuesta fija) con SPY de argumentos. Supone
// que el middleware llama a IsUserAssociatedWithComplex exactamente con el
// usuario y el complejo ya parseados.
type stubComplexChecker struct {
	ok         bool
	err        error
	calls      int
	gotUser    uuid.UUID
	gotComplex uuid.UUID
}

func (s *stubComplexChecker) IsUserAssociatedWithComplex(_ context.Context, u, c uuid.UUID) (bool, error) {
	s.calls++
	s.gotUser, s.gotComplex = u, c
	return s.ok, s.err
}

// stubUnitChecker: igual que el anterior, para unidades.
type stubUnitChecker struct {
	ok      bool
	err     error
	calls   int
	gotUser uuid.UUID
	gotUnit uuid.UUID
}

func (s *stubUnitChecker) IsUserAssociatedWithUnit(_ context.Context, u, unit uuid.UUID) (bool, error) {
	s.calls++
	s.gotUser, s.gotUnit = u, unit
	return s.ok, s.err
}

// stubStaffChecker: STUB + SPY para RequireStaffMiddleware (ramas que no cubren
// los casos existentes MW-E5..E7).
type stubStaffChecker struct {
	ok      bool
	calls   int
	gotRole string
}

func (s *stubStaffChecker) IsStaffOfComplex(_ context.Context, _, _ uuid.UUID, role string) (bool, error) {
	s.calls++
	s.gotRole = role
	return s.ok, nil
}

// ---------------------------------------------------------------------------
// Utilidades
// ---------------------------------------------------------------------------

func enc(v any) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hs256Sig(msg, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// forgeToken fabrica a mano un token con la cabecera y el payload dados, firmado
// con HMAC-SHA256 y la clave indicada (sirve también para alg falsos).
func forgeToken(header map[string]any, claims map[string]any, secret string) string {
	msg := enc(header) + "." + enc(claims)
	return msg + "." + hs256Sig(msg, secret)
}

func hsClaims(user string, exp time.Time) map[string]any {
	return map[string]any{"user_id": user, "email": "u@x.co", "role": "GUARD", "iat": time.Now().Unix(), "exp": exp.Unix()}
}

func validToken(t *testing.T, user, role, complexID string) string {
	t.Helper()
	tok, err := GenerateJWT(user, "u@x.co", role, complexID)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// errorMessage decodifica el sobre de error del middleware.
func errorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp apperrors.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("cuerpo no es el sobre de error: %q (%v)", rec.Body.String(), err)
	}
	if resp.Success {
		t.Fatalf("success debe ser false: %q", rec.Body.String())
	}
	return resp.Error.Message
}

// ctxWithUser arma un contexto con usuario (y opcionalmente complejo y rol).
func ctxWithUser(user, complexID, role string) context.Context {
	ctx := context.Background()
	if user != "" {
		ctx = context.WithValue(ctx, UserIDKey, user)
	}
	if complexID != "" {
		ctx = context.WithValue(ctx, ComplexIDKey, complexID)
	}
	if role != "" {
		ctx = context.WithValue(ctx, UserRoleKey, role)
	}
	return ctx
}

// withURLParams añade parámetros de ruta de chi al request.
func withURLParams(r *http.Request, kv ...string) *http.Request {
	rc := chi.NewRouteContext()
	for i := 0; i < len(kv); i += 2 {
		rc.URLParams.Add(kv[i], kv[i+1])
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// ---------------------------------------------------------------------------
// MW-01 … MW-06: validación de tokens HS256 y cabecera Authorization
// ---------------------------------------------------------------------------

// Caso MW-01 · HS256 vencido
// Técnica: Valores límite.
// Entrada: GenerateJWTWithExpiry(-1m); exp=now-2s; exp=now+60s
// Debe hacer / esperado: token expired (función y 401 vía middleware, next 0 veces); +60 s válido
func TestMW01_TokenHS256VencidoEsRechazado(t *testing.T) {
	t.Setenv("JWT_SECRET", unitSecret)
	t.Setenv("GATEWAY_SECRET", "")
	user := uuid.NewString()

	generated, err := GenerateJWTWithExpiry(user, "u@x.co", "GUARD", "", -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	hdr := map[string]any{"alg": "HS256", "typ": "JWT"}

	cases := []struct {
		name    string
		token   string
		wantErr string
	}{
		{"MW-01a GenerateJWTWithExpiry -1min", generated, "token expired"},
		{"MW-01b exp 2 s en el pasado", forgeToken(hdr, hsClaims(user, time.Now().Add(-2*time.Second)), unitSecret), "token expired"},
		{"MW-01c exp 60 s en el futuro es valido", forgeToken(hdr, hsClaims(user, time.Now().Add(60*time.Second)), unitSecret), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Nivel función
			claims, err := ValidateJWT(tc.token)
			if tc.wantErr == "" {
				if err != nil || claims.UserID != user {
					t.Fatalf("claims=%+v err=%v", claims, err)
				}
			} else if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("err=%v, quiero %q", err, tc.wantErr)
			}
			// Nivel middleware
			next := &spyNext{}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			rec := serve(AuthMiddleware(next), req)
			if tc.wantErr == "" {
				if rec.Code != http.StatusOK || next.calls != 1 {
					t.Fatalf("status=%d calls=%d", rec.Code, next.calls)
				}
				return
			}
			if rec.Code != http.StatusUnauthorized || next.calls != 0 {
				t.Fatalf("status=%d calls=%d, quiero 401 y 0", rec.Code, next.calls)
			}
			if msg := errorMessage(t, rec); !strings.Contains(msg, tc.wantErr) {
				t.Fatalf("mensaje %q debe contener %q", msg, tc.wantErr)
			}
		})
	}
}

// Caso MW-02 · Firma alterada
// Técnica: Adivinación de errores.
// Entrada: payload con rol ADMIN y firma original; último carácter de firma cambiado; otra clave;
// firma vacía
// Debe hacer / esperado: invalid signature; control positivo del token íntegro
func TestMW02_FirmaAlteradaEsRechazada(t *testing.T) {
	t.Setenv("JWT_SECRET", unitSecret)
	user := uuid.NewString()
	good := validToken(t, user, "RESIDENT", "")
	parts := strings.Split(good, ".")

	// Escalada de privilegios: se cambia el rol en el payload conservando la firma original.
	elevated := hsClaims(user, time.Now().Add(time.Hour))
	elevated["role"] = "ADMIN"
	tamperedPayload := parts[0] + "." + enc(elevated) + "." + parts[2]

	// Último carácter de la firma cambiado.
	last := parts[2][len(parts[2])-1]
	repl := byte('A')
	if last == 'A' {
		repl = 'B'
	}
	tamperedSig := parts[0] + "." + parts[1] + "." + parts[2][:len(parts[2])-1] + string(repl)

	cases := []struct {
		name  string
		token string
	}{
		{"MW-02a payload alterado con firma original", tamperedPayload},
		{"MW-02b ultimo caracter de la firma alterado", tamperedSig},
		{"MW-02c firmado con otra clave", forgeToken(map[string]any{"alg": "HS256", "typ": "JWT"}, hsClaims(user, time.Now().Add(time.Hour)), "otra-clave")},
		{"MW-02d firma vacia", parts[0] + "." + parts[1] + "."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ValidateJWT(tc.token); err == nil || err.Error() != "invalid signature" {
				t.Fatalf("err=%v, quiero invalid signature", err)
			}
		})
	}
	// Control positivo: el token íntegro sí valida (el oráculo no es trivial).
	if c, err := ValidateJWT(good); err != nil || c.Role != "RESIDENT" {
		t.Fatalf("token integro: %+v %v", c, err)
	}
}

// Caso MW-03 · alg/typ no permitidos
// Técnica: Partición en clases de equivalencia.
// Entrada: none sin firma, HS512 con firma válida, hs256, alg ausente, typ:JWS, typ ausente, RS256
// sin configurar
// Debe hacer / esperado: unsupported JWT algorithm / unsupported JWT type; typ ausente aceptado;
// RS256 sin configurar rechazado
func TestMW03_AlgoritmoYTipoNoPermitidos(t *testing.T) {
	t.Setenv("JWT_SECRET", unitSecret)
	user := uuid.NewString()
	claims := hsClaims(user, time.Now().Add(time.Hour))

	cases := []struct {
		name    string
		token   string
		wantErr string // "" = aceptado
	}{
		{"MW-03a alg none sin firma", enc(map[string]any{"alg": "none", "typ": "JWT"}) + "." + enc(claims) + ".", "unsupported JWT algorithm"},
		{"MW-03b alg HS512 con firma valida", forgeToken(map[string]any{"alg": "HS512", "typ": "JWT"}, claims, unitSecret), "unsupported JWT algorithm"},
		{"MW-03c alg en minusculas hs256", forgeToken(map[string]any{"alg": "hs256", "typ": "JWT"}, claims, unitSecret), "unsupported JWT algorithm"},
		{"MW-03d alg ausente", forgeToken(map[string]any{"typ": "JWT"}, claims, unitSecret), "unsupported JWT algorithm"},
		{"MW-03e typ JWS", forgeToken(map[string]any{"alg": "HS256", "typ": "JWS"}, claims, unitSecret), "unsupported JWT type"},
		{"MW-03f typ ausente se acepta", forgeToken(map[string]any{"alg": "HS256"}, claims, unitSecret), ""},
		{"MW-03g RS256 sin configurar", forgeToken(map[string]any{"alg": "RS256", "typ": "JWT"}, claims, unitSecret), "RS256 not configured"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := ValidateJWT(tc.token)
			if tc.wantErr == "" {
				if err != nil || c.UserID != user {
					t.Fatalf("c=%+v err=%v", c, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("token aceptado, quiero error %q", tc.wantErr)
			}
			if tc.wantErr == "RS256 not configured" {
				// Mensaje exacto de rs256.go no es parte del contrato de C8: basta con rechazo y sin claims.
				if c != nil {
					t.Fatalf("no debe devolver claims: %+v", c)
				}
				return
			}
			if err.Error() != tc.wantErr || c != nil {
				t.Fatalf("err=%v claims=%+v, quiero %q", err, c, tc.wantErr)
			}
		})
	}
}

// Caso MW-04 · Formato
// Técnica: Partición en clases de equivalencia, Valores límite.
// Entrada: "", 1, 2, 4 partes; cabecera no base64 / no JSON; payload no base64 / no JSON
// Debe hacer / esperado: invalid token format / failed to decode header / failed to parse header /
// failed to decode payload / failed to parse claims
func TestMW04_FormatoDeTokenInvalido(t *testing.T) {
	t.Setenv("JWT_SECRET", unitSecret)
	okHdr := enc(map[string]any{"alg": "HS256", "typ": "JWT"})
	okPayload := enc(hsClaims(uuid.NewString(), time.Now().Add(time.Hour)))

	cases := []struct {
		name    string
		token   string
		wantErr string // subcadena
	}{
		{"MW-04a vacio", "", "invalid token format"},
		{"MW-04b una parte", "abc", "invalid token format"},
		{"MW-04c dos partes", okHdr + "." + okPayload, "invalid token format"},
		{"MW-04d cuatro partes", okHdr + "." + okPayload + ".x.y", "invalid token format"},
		{"MW-04e cabecera no es base64", "***." + okPayload + ".sig", "failed to decode header"},
		{"MW-04f cabecera base64 pero no JSON", base64.RawURLEncoding.EncodeToString([]byte("no-json")) + "." + okPayload + ".sig", "failed to parse header"},
		{"MW-04g payload no es base64", okHdr + ".***.sig", "failed to decode payload"},
		{"MW-04h payload base64 pero no JSON", okHdr + "." + base64.RawURLEncoding.EncodeToString([]byte("no-json")) + ".sig", "failed to parse claims"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := ValidateJWT(tc.token)
			if err == nil || c != nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("claims=%+v err=%v, quiero error con %q", c, err, tc.wantErr)
			}
		})
	}
}

// Caso MW-05 · JWT_SECRET vacío (fail-closed)
// Técnica: Adivinación de errores.
// Entrada: token forjado con clave "" ; GenerateJWT
// Debe hacer / esperado: JWT_SECRET not configured en validar y generar; 401 en middleware
func TestMW05_SinJWTSecretNoSeEmiteNiSeAcepta(t *testing.T) {
	user := uuid.NewString()
	// Token firmado con la clave vacía: el atacante supone que "sin secreto" valida cualquier cosa.
	forged := forgeToken(map[string]any{"alg": "HS256", "typ": "JWT"}, hsClaims(user, time.Now().Add(time.Hour)), "")

	t.Setenv("JWT_SECRET", "")
	if _, err := ValidateJWT(forged); err == nil || err.Error() != "JWT_SECRET not configured" {
		t.Fatalf("validar: err=%v, quiero JWT_SECRET not configured", err)
	}
	if tok, err := GenerateJWT(user, "u@x.co", "GUARD", ""); err == nil || tok != "" || err.Error() != "JWT_SECRET not configured" {
		t.Fatalf("generar: tok=%q err=%v", tok, err)
	}

	next := &spyNext{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+forged)
	rec := serve(AuthMiddleware(next), req)
	if rec.Code != http.StatusUnauthorized || next.calls != 0 {
		t.Fatalf("status=%d calls=%d, quiero 401 y 0", rec.Code, next.calls)
	}
}

// Caso MW-06 · Cabecera Authorization
// Técnica: Partición en clases de equivalencia.
// Entrada: ausente, Token x, Bearer, Bearer a b, bearer x, Bearer basura, válido
// Debe hacer / esperado: 401 con mensaje exacto; válido → 200 y ctx con user/complex/role/email
func TestMW06_CabeceraAuthorization(t *testing.T) {
	t.Setenv("JWT_SECRET", unitSecret)
	t.Setenv("GATEWAY_SECRET", "")
	user, cx := uuid.NewString(), uuid.NewString()
	good := validToken(t, user, "GUARD", cx)

	cases := []struct {
		name    string
		header  string
		set     bool
		want    int
		wantMsg string
	}{
		{"MW-06a ausente", "", false, 401, "Missing authorization header"},
		{"MW-06b esquema Token", "Token " + good, true, 401, "Invalid authorization header format"},
		{"MW-06c Bearer sin token", "Bearer", true, 401, "Invalid authorization header format"},
		{"MW-06d Bearer con dos tokens", "Bearer a b", true, 401, "Invalid authorization header format"},
		{"MW-06e esquema en minusculas", "bearer " + good, true, 401, "Invalid authorization header format"},
		{"MW-06f Bearer con token basura", "Bearer basura", true, 401, "Invalid token:"},
		{"MW-06g Bearer valido", "Bearer " + good, true, 200, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := &spyNext{}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.set {
				req.Header.Set("Authorization", tc.header)
			}
			rec := serve(AuthMiddleware(next), req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d, quiero %d", rec.Code, tc.want)
			}
			if tc.want == 200 {
				if next.calls != 1 {
					t.Fatalf("next calls=%d", next.calls)
				}
				ctx := next.ctx
				if GetUserID(ctx) != user || GetComplexID(ctx) != cx || GetUserRole(ctx) != "GUARD" {
					t.Fatalf("contexto: user=%q complex=%q role=%q", GetUserID(ctx), GetComplexID(ctx), GetUserRole(ctx))
				}
				if email, _ := ctx.Value(EmailKey).(string); email != "u@x.co" {
					t.Fatalf("email=%q", email)
				}
				return
			}
			if next.calls != 0 {
				t.Fatalf("next no debe ejecutarse")
			}
			if msg := errorMessage(t, rec); !strings.Contains(msg, tc.wantMsg) {
				t.Fatalf("mensaje %q debe contener %q", msg, tc.wantMsg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MW-07: identidad inyectada por el gateway (complementa MW-E1..E4)
// ---------------------------------------------------------------------------

// Caso MW-07 · Identidad del gateway
// Técnica: Tabla de decisión.
// Entrada: secreto errado / prefijo más largo; sin X-User-ID; X-Complex-ID inválido; rol " guard
// "; sin rol; secreto errado + JWT válido; secreto ok + JWT válido
// Debe hacer / esperado: 401 con mensaje; rol normalizado GUARD; sin rol no hay clave en ctx;
// secreto errado no cae al JWT; gateway gana al JWT
func TestMW07_IdentidadDelGateway(t *testing.T) {
	t.Setenv("GATEWAY_SECRET", "gw-unit-secret")
	t.Setenv("JWT_SECRET", unitSecret)
	user, cx := uuid.NewString(), uuid.NewString()
	validJWT := validToken(t, uuid.NewString(), "ADMIN", "")

	cases := []struct {
		name     string
		headers  map[string]string
		want     int
		wantMsg  string
		wantUser string
		wantCx   string
		wantRole string
	}{
		{"MW-07a secreto incorrecto", map[string]string{"X-Gateway-Secret": "otro", "X-User-ID": user}, 401, "request did not come through the gateway", "", "", ""},
		{"MW-07b secreto con prefijo correcto pero mas largo", map[string]string{"X-Gateway-Secret": "gw-unit-secret-x", "X-User-ID": user}, 401, "request did not come through the gateway", "", "", ""},
		{"MW-07c secreto correcto sin X-User-ID", map[string]string{"X-Gateway-Secret": "gw-unit-secret"}, 401, "authentication required", "", "", ""},
		{"MW-07d X-Complex-ID invalido", map[string]string{"X-Gateway-Secret": "gw-unit-secret", "X-User-ID": user, "X-Complex-ID": "no-uuid"}, 401, "invalid complex ID format", "", "", ""},
		{"MW-07e rol con espacios y minusculas se normaliza", map[string]string{"X-Gateway-Secret": "gw-unit-secret", "X-User-ID": user, "X-Complex-ID": cx, "X-Complex-Role": "  guard "}, 200, "", user, cx, "GUARD"},
		{"MW-07f sin rol no hay rol en contexto", map[string]string{"X-Gateway-Secret": "gw-unit-secret", "X-User-ID": user, "X-Complex-ID": cx}, 200, "", user, cx, ""},
		{"MW-07g secreto errado no cae al JWT valido", map[string]string{"X-Gateway-Secret": "otro", "X-User-ID": user, "Authorization": "Bearer " + validJWT}, 401, "request did not come through the gateway", "", "", ""},
		{"MW-07h el gateway gana sobre un JWT valido", map[string]string{"X-Gateway-Secret": "gw-unit-secret", "X-User-ID": user, "Authorization": "Bearer " + validJWT}, 200, "", user, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := &spyNext{}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := serve(AuthMiddleware(next), req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d, quiero %d", rec.Code, tc.want)
			}
			if tc.want != 200 {
				if next.calls != 0 {
					t.Fatal("next no debe ejecutarse")
				}
				if msg := errorMessage(t, rec); msg != tc.wantMsg {
					t.Fatalf("mensaje=%q, quiero %q", msg, tc.wantMsg)
				}
				return
			}
			if next.calls != 1 {
				t.Fatalf("next calls=%d", next.calls)
			}
			if GetUserID(next.ctx) != tc.wantUser || GetComplexID(next.ctx) != tc.wantCx || GetUserRole(next.ctx) != tc.wantRole {
				t.Fatalf("ctx: user=%q cx=%q role=%q", GetUserID(next.ctx), GetComplexID(next.ctx), GetUserRole(next.ctx))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MW-08 / MW-08b: RequireComplexAccessMiddleware
// ---------------------------------------------------------------------------

// Caso MW-08 · RequireComplexAccess
// Técnica: Tabla de decisión.
// Entrada: sin usuario; usuario no UUID; sin complejo; complejo inválido; checker error (también
// con ok=true); no pertenece; pertenece
// Debe hacer / esperado: 401/401/400/400/500/500/403/200; checker 0 llamadas en los 4 primeros; el
// checker recibe user y complex parseados; ctx con ComplexIDKey
func TestMW08_RequireComplexAccess(t *testing.T) {
	user, cx := uuid.New(), uuid.New()
	boom := errors.New("db down")

	cases := []struct {
		name          string
		ctxUser       string
		complexHeader string
		checker       *stubComplexChecker
		want          int
		wantMsg       string
		wantCalls     int
	}{
		{"MW-08a sin usuario", "", cx.String(), &stubComplexChecker{ok: true}, 401, "authentication required", 0},
		{"MW-08b usuario no UUID", "admin", cx.String(), &stubComplexChecker{ok: true}, 401, "invalid user ID", 0},
		{"MW-08c sin complejo", user.String(), "", &stubComplexChecker{ok: true}, 400, "complex ID is required", 0},
		{"MW-08d complejo invalido", user.String(), "xyz", &stubComplexChecker{ok: true}, 400, "invalid complex ID format", 0},
		{"MW-08e error del checker", user.String(), cx.String(), &stubComplexChecker{err: boom}, 500, "failed to verify complex access", 1},
		{"MW-08f error del checker con ok=true sigue siendo 500", user.String(), cx.String(), &stubComplexChecker{ok: true, err: boom}, 500, "failed to verify complex access", 1},
		{"MW-08g no pertenece", user.String(), cx.String(), &stubComplexChecker{ok: false}, 403, "you do not have access to this complex", 1},
		{"MW-08h pertenece", user.String(), cx.String(), &stubComplexChecker{ok: true}, 200, "", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := &spyNext{}
			req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctxWithUser(tc.ctxUser, "", ""))
			if tc.complexHeader != "" {
				req.Header.Set("X-Complex-ID", tc.complexHeader)
			}
			rec := serve(RequireComplexAccessMiddleware(tc.checker)(next), req)
			if rec.Code != tc.want || tc.checker.calls != tc.wantCalls {
				t.Fatalf("status=%d checker.calls=%d, quiero %d/%d", rec.Code, tc.checker.calls, tc.want, tc.wantCalls)
			}
			if tc.want != 200 {
				if next.calls != 0 {
					t.Fatal("next no debe ejecutarse")
				}
				if msg := errorMessage(t, rec); msg != tc.wantMsg {
					t.Fatalf("mensaje=%q, quiero %q", msg, tc.wantMsg)
				}
				return
			}
			if next.calls != 1 || GetComplexID(next.ctx) != cx.String() {
				t.Fatalf("next calls=%d complex en ctx=%q", next.calls, GetComplexID(next.ctx))
			}
			if tc.checker.gotUser != user || tc.checker.gotComplex != cx {
				t.Fatalf("el checker recibio user=%s complex=%s", tc.checker.gotUser, tc.checker.gotComplex)
			}
			if GetUserID(next.ctx) != user.String() {
				t.Fatal("el usuario debe conservarse en el contexto")
			}
		})
	}
}

// Caso MW-08b · Precedencia extractComplexID
// Técnica: Tabla de decisión.
// Entrada: 6 fuentes con UUID distintos (11 combinaciones)
// Debe hacer / esperado: Gana la primera no vacía (el checker recibe ese UUID); primera inválida →
// 400 aunque haya posterior válida; posteriores inválidas ignoradas si la primera es válida;
// ninguna → complex ID is required
func TestMW08b_PrecedenciaDeExtractComplexID(t *testing.T) {
	user := uuid.New().String()
	ids := make([]string, 7)
	for i := range ids {
		ids[i] = uuid.NewString()
	}

	type src struct {
		urlComplexId, urlComplex_id, claim, header, qComplex_id, qComplexId string
	}
	cases := []struct {
		name    string
		in      src
		want    string // UUID esperado en el checker; "" => 400
		wantMsg string
	}{
		{"MW-08b-1 todos presentes gana el param complexId", src{ids[0], ids[1], ids[2], ids[3], ids[4], ids[5]}, ids[0], ""},
		{"MW-08b-2 sin complexId gana complex_id", src{"", ids[1], ids[2], ids[3], ids[4], ids[5]}, ids[1], ""},
		{"MW-08b-3 sin params gana el claim", src{"", "", ids[2], ids[3], ids[4], ids[5]}, ids[2], ""},
		{"MW-08b-4 sin claim gana la cabecera", src{"", "", "", ids[3], ids[4], ids[5]}, ids[3], ""},
		{"MW-08b-5 sin cabecera gana query complex_id", src{"", "", "", "", ids[4], ids[5]}, ids[4], ""},
		{"MW-08b-6 solo query complexId", src{"", "", "", "", "", ids[5]}, ids[5], ""},
		{"MW-08b-7 param invalido gana aunque haya claim valido", src{"basura", "", ids[2], "", "", ""}, "", "invalid complex ID format"},
		{"MW-08b-8 claim invalido gana aunque haya cabecera valida", src{"", "", "basura", ids[3], "", ""}, "", "invalid complex ID format"},
		{"MW-08b-9 cabecera invalida gana aunque haya query valida", src{"", "", "", "basura", ids[4], ""}, "", "invalid complex ID format"},
		{"MW-08b-10 candidato valido anterior ignora los invalidos posteriores", src{"", "", ids[2], "basura", "basura", "basura"}, ids[2], ""},
		{"MW-08b-11 ninguno", src{}, "", "complex ID is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checker := &stubComplexChecker{ok: true}
			next := &spyNext{}
			q := ""
			add := func(k, v string) {
				if v != "" {
					if q != "" {
						q += "&"
					}
					q += k + "=" + v
				}
			}
			add("complex_id", tc.in.qComplex_id)
			add("complexId", tc.in.qComplexId)
			req := httptest.NewRequest(http.MethodGet, "/?"+q, nil)
			req = req.WithContext(ctxWithUser(user, tc.in.claim, ""))
			if tc.in.header != "" {
				req.Header.Set("X-Complex-ID", tc.in.header)
			}
			var params []string
			if tc.in.urlComplexId != "" {
				params = append(params, "complexId", tc.in.urlComplexId)
			}
			if tc.in.urlComplex_id != "" {
				params = append(params, "complex_id", tc.in.urlComplex_id)
			}
			req = withURLParams(req, params...)

			rec := serve(RequireComplexAccessMiddleware(checker)(next), req)
			if tc.want == "" {
				if rec.Code != http.StatusBadRequest || checker.calls != 0 || next.calls != 0 {
					t.Fatalf("status=%d checker=%d next=%d, quiero 400/0/0", rec.Code, checker.calls, next.calls)
				}
				if msg := errorMessage(t, rec); msg != tc.wantMsg {
					t.Fatalf("mensaje=%q, quiero %q", msg, tc.wantMsg)
				}
				return
			}
			if rec.Code != 200 || checker.gotComplex.String() != tc.want || GetComplexID(next.ctx) != tc.want {
				t.Fatalf("status=%d checker.complex=%s ctx=%q, quiero %s", rec.Code, checker.gotComplex, GetComplexID(next.ctx), tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MW-09: RequireUnitAccessMiddleware
// ---------------------------------------------------------------------------

// Caso MW-09 · RequireUnitAccess
// Técnica: Tabla de decisión.
// Entrada: análogo a MW-08 + las 4 fuentes de unidad y precedencia
// Debe hacer / esperado: 401/401/400/400/500/403/200; mapeo de cada fuente
func TestMW09_RequireUnitAccess(t *testing.T) {
	user, unit, other := uuid.New(), uuid.New(), uuid.New()
	boom := errors.New("db down")

	cases := []struct {
		name      string
		ctxUser   string
		urlUnit   string // param "unitId"
		urlUnit2  string // param "unit_id"
		qUnit     string // query unit_id
		qUnit2    string // query unitId
		checker   *stubUnitChecker
		want      int
		wantMsg   string
		wantCalls int
		wantUnit  uuid.UUID
	}{
		{"MW-09a sin usuario", "", unit.String(), "", "", "", &stubUnitChecker{ok: true}, 401, "authentication required", 0, uuid.Nil},
		{"MW-09b usuario no UUID", "admin", unit.String(), "", "", "", &stubUnitChecker{ok: true}, 401, "authentication required", 0, uuid.Nil},
		{"MW-09c sin unidad", user.String(), "", "", "", "", &stubUnitChecker{ok: true}, 400, "unit ID is required", 0, uuid.Nil},
		{"MW-09d unidad invalida", user.String(), "xyz", "", "", "", &stubUnitChecker{ok: true}, 400, "invalid unit ID format", 0, uuid.Nil},
		{"MW-09e error del checker", user.String(), unit.String(), "", "", "", &stubUnitChecker{err: boom}, 500, "failed to verify unit access", 1, unit},
		{"MW-09f no pertenece", user.String(), unit.String(), "", "", "", &stubUnitChecker{ok: false}, 403, "you do not have access to this unit", 1, unit},
		{"MW-09g pertenece (param unitId)", user.String(), unit.String(), "", "", "", &stubUnitChecker{ok: true}, 200, "", 1, unit},
		{"MW-09h param unit_id", user.String(), "", unit.String(), "", "", &stubUnitChecker{ok: true}, 200, "", 1, unit},
		{"MW-09i query unit_id", user.String(), "", "", unit.String(), "", &stubUnitChecker{ok: true}, 200, "", 1, unit},
		{"MW-09j query unitId", user.String(), "", "", "", unit.String(), &stubUnitChecker{ok: true}, 200, "", 1, unit},
		{"MW-09k precedencia: el param de URL gana a la query", user.String(), unit.String(), "", other.String(), "", &stubUnitChecker{ok: true}, 200, "", 1, unit},
		{"MW-09l primer candidato invalido gana aunque la query sea valida", user.String(), "basura", "", unit.String(), "", &stubUnitChecker{ok: true}, 400, "invalid unit ID format", 0, uuid.Nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := &spyNext{}
			q := ""
			if tc.qUnit != "" {
				q += "unit_id=" + tc.qUnit + "&"
			}
			if tc.qUnit2 != "" {
				q += "unitId=" + tc.qUnit2
			}
			req := httptest.NewRequest(http.MethodGet, "/?"+q, nil).WithContext(ctxWithUser(tc.ctxUser, "", ""))
			var params []string
			if tc.urlUnit != "" {
				params = append(params, "unitId", tc.urlUnit)
			}
			if tc.urlUnit2 != "" {
				params = append(params, "unit_id", tc.urlUnit2)
			}
			req = withURLParams(req, params...)

			rec := serve(RequireUnitAccessMiddleware(tc.checker)(next), req)
			if rec.Code != tc.want || tc.checker.calls != tc.wantCalls {
				t.Fatalf("status=%d checker.calls=%d, quiero %d/%d", rec.Code, tc.checker.calls, tc.want, tc.wantCalls)
			}
			if tc.wantCalls > 0 && (tc.checker.gotUnit != tc.wantUnit || tc.checker.gotUser != user) {
				t.Fatalf("checker recibio user=%s unit=%s", tc.checker.gotUser, tc.checker.gotUnit)
			}
			if tc.want != 200 {
				if next.calls != 0 {
					t.Fatal("next no debe ejecutarse")
				}
				if msg := errorMessage(t, rec); msg != tc.wantMsg {
					t.Fatalf("mensaje=%q, quiero %q", msg, tc.wantMsg)
				}
				return
			}
			if next.calls != 1 {
				t.Fatalf("next calls=%d", next.calls)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MW-10: RequireRoleMiddleware
// ---------------------------------------------------------------------------

// Caso MW-10 · RequireRole
// Técnica: Partición en clases de equivalencia.
// Entrada: rol en lista (1.º/2.º), fuera, sin rol, lista vacía, guard vs GUARD
// Debe hacer / esperado: 200 / 403 con mensaje; next 0 veces en 403
func TestMW10_RequireRole(t *testing.T) {
	cases := []struct {
		name    string
		role    string
		allowed []string
		want    int
	}{
		{"MW-10a rol en la lista", "GUARD", []string{"GUARD", "ADMIN"}, 200},
		{"MW-10b segundo de la lista", "ADMIN", []string{"GUARD", "ADMIN"}, 200},
		{"MW-10c rol fuera de la lista", "RESIDENT", []string{"GUARD", "ADMIN"}, 403},
		{"MW-10d sin rol en contexto", "", []string{"GUARD", "ADMIN"}, 403},
		{"MW-10e lista vacia", "ADMIN", nil, 403},
		{"MW-10f distingue mayusculas", "guard", []string{"GUARD"}, 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := &spyNext{}
			req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctxWithUser("", "", tc.role))
			rec := serve(RequireRoleMiddleware(tc.allowed...)(next), req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d, quiero %d", rec.Code, tc.want)
			}
			if tc.want == 200 && next.calls != 1 {
				t.Fatalf("next calls=%d", next.calls)
			}
			if tc.want == 403 {
				if next.calls != 0 {
					t.Fatal("next no debe ejecutarse")
				}
				if msg := errorMessage(t, rec); msg != "your role does not have access to this operation" {
					t.Fatalf("mensaje=%q", msg)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MW-11: OptionalAuthMiddleware
// ---------------------------------------------------------------------------

// Caso MW-11 · OptionalAuth
// Técnica: Partición en clases de equivalencia.
// Entrada: sin cabecera, basura, vencido, firma alterada, esquema Token, válido
// Debe hacer / esperado: siempre 200 y next 1 vez; usuario en ctx solo con token válido; sin
// identidad parcial en los demás
func TestMW11_OptionalAuth(t *testing.T) {
	t.Setenv("JWT_SECRET", unitSecret)
	t.Setenv("GATEWAY_SECRET", "")
	user, cx := uuid.NewString(), uuid.NewString()
	good := validToken(t, user, "GUARD", cx)
	expired, _ := GenerateJWTWithExpiry(user, "u@x.co", "GUARD", cx, -time.Minute)
	tampered := good[:len(good)-2] + "AA"
	if tampered == good {
		tampered = good[:len(good)-2] + "BB"
	}

	cases := []struct {
		name     string
		header   string
		set      bool
		wantUser string
	}{
		{"MW-11a sin cabecera", "", false, ""},
		{"MW-11b token basura", "Bearer basura", true, ""},
		{"MW-11c token vencido", "Bearer " + expired, true, ""},
		{"MW-11d firma alterada", "Bearer " + tampered, true, ""},
		{"MW-11e esquema distinto de Bearer", "Token " + good, true, ""},
		{"MW-11f token valido", "Bearer " + good, true, user},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := &spyNext{}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.set {
				req.Header.Set("Authorization", tc.header)
			}
			rec := serve(OptionalAuthMiddleware(next), req)
			if rec.Code != http.StatusOK || next.calls != 1 {
				t.Fatalf("status=%d next calls=%d, quiero 200/1", rec.Code, next.calls)
			}
			if got := GetUserID(next.ctx); got != tc.wantUser {
				t.Fatalf("usuario en ctx=%q, quiero %q", got, tc.wantUser)
			}
			if tc.wantUser != "" && (GetComplexID(next.ctx) != cx || GetUserRole(next.ctx) != "GUARD") {
				t.Fatalf("ctx complejo=%q rol=%q", GetComplexID(next.ctx), GetUserRole(next.ctx))
			}
			if tc.wantUser == "" && (GetComplexID(next.ctx) != "" || GetUserRole(next.ctx) != "") {
				t.Fatalf("no debe haber identidad parcial: complejo=%q rol=%q", GetComplexID(next.ctx), GetUserRole(next.ctx))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MW-12: ApplyEnvironmentAuth
// ---------------------------------------------------------------------------

// Caso MW-12 · ApplyEnvironmentAuth
// Técnica: Partición en clases de equivalencia.
// Entrada: ENV "", production, staging, Development, development × sin/ inválido/ válido token
// (router chi real)
// Debe hacer / esperado: todo salvo development exacto exige token (401); development: pasa sin
// token, token inválido sin usuario, válido con usuario
func TestMW12_ApplyEnvironmentAuth(t *testing.T) {
	t.Setenv("JWT_SECRET", unitSecret)
	t.Setenv("GATEWAY_SECRET", "")
	user := uuid.NewString()
	good := validToken(t, user, "GUARD", "")

	cases := []struct {
		name       string
		env        string
		token      string // "" = sin cabecera
		want       int
		wantUser   string
		wantCalled bool
	}{
		{"MW-12a ENV vacio exige token", "", "", 401, "", false},
		{"MW-12b production exige token", "production", "", 401, "", false},
		{"MW-12c production con token valido", "production", good, 200, user, true},
		{"MW-12d valor desconocido staging exige token", "staging", "", 401, "", false},
		{"MW-12e Development con mayuscula NO es development (fail-closed)", "Development", "", 401, "", false},
		{"MW-12f development sin token pasa", "development", "", 200, "", true},
		{"MW-12g development con token invalido pasa sin usuario", "development", "basura", 200, "", true},
		{"MW-12h development con token valido trae usuario", "development", good, 200, user, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENV", tc.env)
			next := &spyNext{}
			r := chi.NewRouter()
			ApplyEnvironmentAuth(r)
			r.Get("/", next.ServeHTTP)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			rec := serve(r, req)
			if rec.Code != tc.want || (next.calls == 1) != tc.wantCalled {
				t.Fatalf("status=%d next calls=%d, quiero %d/%v", rec.Code, next.calls, tc.want, tc.wantCalled)
			}
			if tc.wantCalled && GetUserID(next.ctx) != tc.wantUser {
				t.Fatalf("usuario en ctx=%q, quiero %q", GetUserID(next.ctx), tc.wantUser)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MW-13 / MW-14: casos adicionales (helpers de contexto, generación de JWT, RequireStaff)
// ---------------------------------------------------------------------------

// Caso MW-13 · Generación JWT y getters de contexto (caso adicional, no estaba en el plan)
// Técnica: Partición en clases de equivalencia.
// Entrada: claims del token, iat/exp ≈ 24 h (±2 s), ctx vacío, valores no UUID, valor no string
// bajo la clave
// Debe hacer / esperado: claims coherentes; getters devuelven "" / error
func TestMW13_GeneracionDeJWTYGettersDeContexto(t *testing.T) {
	t.Setenv("JWT_SECRET", unitSecret)
	user, cx := uuid.NewString(), uuid.NewString()

	before := time.Now().Unix()
	tok := validToken(t, user, "ADMIN", cx)
	after := time.Now().Unix()
	c, err := ValidateJWT(tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.UserID != user || c.Email != "u@x.co" || c.Role != "ADMIN" || c.ComplexID != cx {
		t.Fatalf("claims=%+v", c)
	}
	if c.IssuedAt < before || c.IssuedAt > after {
		t.Fatalf("iat=%d fuera de [%d,%d]", c.IssuedAt, before, after)
	}
	// Margen de ±2 s alrededor de 24 h por redondeo a segundos.
	if d := c.ExpiresAt - c.IssuedAt; d < int64(24*time.Hour/time.Second)-2 || d > int64(24*time.Hour/time.Second)+2 {
		t.Fatalf("exp-iat=%d s, quiero ~86400", d)
	}

	empty := context.Background()
	if GetUserID(empty) != "" || GetComplexID(empty) != "" || GetUserRole(empty) != "" {
		t.Fatal("los getters deben devolver cadena vacia sin valor")
	}
	if _, err := GetUserIDAsUUID(empty); err == nil {
		t.Fatal("GetUserIDAsUUID sin usuario debe fallar")
	}
	if _, err := GetComplexIDAsUUID(empty); err == nil {
		t.Fatal("GetComplexIDAsUUID sin complejo debe fallar")
	}
	if _, err := GetUserIDAsUUID(ctxWithUser("no-uuid", "", "")); err == nil {
		t.Fatal("GetUserIDAsUUID con valor no UUID debe fallar")
	}
	if _, err := GetComplexIDAsUUID(ctxWithUser("", "no-uuid", "")); err == nil {
		t.Fatal("GetComplexIDAsUUID con valor no UUID debe fallar")
	}
	// Un valor de tipo distinto bajo la misma clave no se interpreta como identidad.
	if GetUserID(context.WithValue(empty, UserIDKey, 42)) != "" {
		t.Fatal("un valor no string no debe contarse como usuario")
	}
	got, err := GetUserIDAsUUID(ctxWithUser(user, "", ""))
	if err != nil || got.String() != user {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

// Caso MW-14 · Ramas de RequireStaff no cubiertas por E5-E7 (extra)
// Técnica: Tabla de decisión.
// Entrada: sin usuario, usuario no UUID, complejo no UUID, no staff, staff
// Debe hacer / esperado: 401/401/400/403 (mensaje you are not GUARD of this complex)/200; el
// checker recibe el rol del claim
func TestMW14_RequireStaffRamasRestantes(t *testing.T) {
	user, cx := uuid.NewString(), uuid.NewString()
	cases := []struct {
		name      string
		ctx       context.Context
		checker   *stubStaffChecker
		want      int
		wantCalls int
	}{
		{"MW-14a sin usuario", ctxWithUser("", cx, "GUARD"), &stubStaffChecker{ok: true}, 401, 0},
		{"MW-14b usuario no UUID", ctxWithUser("x", cx, "GUARD"), &stubStaffChecker{ok: true}, 401, 0},
		{"MW-14c complejo no UUID en contexto", ctxWithUser(user, "x", "GUARD"), &stubStaffChecker{ok: true}, 400, 0},
		{"MW-14d no es staff del complejo", ctxWithUser(user, cx, "GUARD"), &stubStaffChecker{ok: false}, 403, 1},
		{"MW-14e es staff", ctxWithUser(user, cx, "ADMIN"), &stubStaffChecker{ok: true}, 200, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := &spyNext{}
			rec := serve(RequireStaffMiddleware(tc.checker, "GUARD", "ADMIN")(next), httptest.NewRequest(http.MethodGet, "/", nil).WithContext(tc.ctx))
			if rec.Code != tc.want || tc.checker.calls != tc.wantCalls {
				t.Fatalf("status=%d checker.calls=%d, quiero %d/%d", rec.Code, tc.checker.calls, tc.want, tc.wantCalls)
			}
			if (tc.want == 200) != (next.calls == 1) {
				t.Fatalf("next calls=%d con status %d", next.calls, rec.Code)
			}
			if tc.wantCalls == 1 && tc.checker.gotRole != GetUserRole(tc.ctx) {
				t.Fatalf("el checker recibio rol %q, quiero el del claim %q", tc.checker.gotRole, GetUserRole(tc.ctx))
			}
			if tc.want == 403 {
				if msg := errorMessage(t, rec); msg != "you are not GUARD of this complex" {
					t.Fatalf("mensaje=%q", msg)
				}
			}
		})
	}
}
