package authn

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	issuer   = "http://auth:8081"
	audience = "superapp-api"
	kid      = "k1"
	userID   = "10000000-0000-0000-0000-000000000001"
	sid      = "session-1"
	devKey   = "dev-hs256-secret-for-tests"
)

// RSA keys are slow to generate; the tests share them.
var (
	keyOnce          sync.Once
	mainKey, otherKy *rsa.PrivateKey
	smallKey         *rsa.PrivateKey
)

func keys(t *testing.T) (*rsa.PrivateKey, *rsa.PrivateKey, *rsa.PrivateKey) {
	t.Helper()
	keyOnce.Do(func() {
		mainKey, _ = rsa.GenerateKey(rand.Reader, 2048)
		otherKy, _ = rsa.GenerateKey(rand.Reader, 2048)
		smallKey, _ = rsa.GenerateKey(rand.Reader, 1024)
	})
	return mainKey, otherKy, smallKey
}

type jwk map[string]string

func rsaJWK(id string, k *rsa.PublicKey) jwk {
	return jwk{
		"kty": "RSA", "use": "sig", "alg": "RS256", "kid": id,
		"n": base64.RawURLEncoding.EncodeToString(k.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes()),
	}
}

// fakeJWKS stands in for auth's /.well-known/jwks.json (stub + spy): it
// serves whatever key set or status the test sets and counts the fetches.
type fakeJWKS struct {
	*httptest.Server
	calls  atomic.Int64
	mu     sync.Mutex
	status int
	body   string
}

func newFakeJWKS(t *testing.T, ks ...jwk) *fakeJWKS {
	f := &fakeJWKS{}
	f.serve(ks...)
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.calls.Add(1)
		f.mu.Lock()
		status, body := f.status, f.body
		f.mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeJWKS) serve(ks ...jwk) {
	b, _ := json.Marshal(map[string]any{"keys": ks})
	f.respond(http.StatusOK, string(b))
}

func (f *fakeJWKS) respond(status int, body string) {
	f.mu.Lock()
	f.status, f.body = status, body
	f.mu.Unlock()
}

func newVerifier(f *fakeJWKS, devSecret string) *Verifier {
	return NewVerifier(Config{JWKSURL: f.URL, Issuer: issuer, Audience: audience, DevHS256Secret: devSecret})
}

func validClaims() jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"iss": issuer, "aud": []string{audience}, "sub": userID, "sid": sid,
		"iat": now.Unix(), "exp": now.Add(15 * time.Minute).Unix(),
	}
}

func signRS(t *testing.T, key *rsa.PrivateKey, keyID string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if keyID != "" {
		tok.Header["kid"] = keyID
	}
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func signHS(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func with(c jwt.MapClaims, kv ...any) jwt.MapClaims {
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] == nil {
			delete(c, kv[i].(string))
		} else {
			c[kv[i].(string)] = kv[i+1]
		}
	}
	return c
}

func mustReject(t *testing.T, v *Verifier, raw string) {
	t.Helper()
	id, err := v.Verify(context.Background(), raw)
	if !errors.Is(err, ErrInvalidToken) || id != (Identity{}) {
		t.Fatalf("Verify = %+v, %v; esperaba ErrInvalidToken", id, err)
	}
}

// Caso AN-01 · token RS256 válido
// Técnica: Partición (clase válida).
// Entrada: token RS256 con iss, aud, sub UUID, sid, iat y exp correctos, firmado con la clave
// publicada en el JWKS (sub en mayúsculas en el segundo subcaso)
// Debe hacer / esperado: Identity con el UserID en forma canónica, el SessionID y DevToken=false.
func TestAN01_TokenRS256Valido(t *testing.T) {
	key, _, _ := keys(t)
	f := newFakeJWKS(t, rsaJWK(kid, &key.PublicKey))
	v := newVerifier(f, "")
	for name, sub := range map[string]string{"AN-01a sub canónico": userID, "AN-01b sub en mayúsculas": strings.ToUpper(userID)} {
		t.Run(name, func(t *testing.T) {
			id, err := v.Verify(context.Background(), signRS(t, key, kid, with(validClaims(), "sub", sub)))
			if err != nil || id != (Identity{UserID: userID, SessionID: sid}) {
				t.Fatalf("Verify = %+v, %v", id, err)
			}
		})
	}
}

// Caso AN-02 · reglas de los claims RS256
// Técnica: Tabla de decisión (un claim inválido por fila) y valores límite del leeway de 5 s.
// Entrada: tokens bien firmados con un claim alterado
// Debe hacer / esperado: solo pasan las filas "acepta"; el resto es ErrInvalidToken.
func TestAN02_ReglasDeLosClaimsRS256(t *testing.T) {
	key, _, _ := keys(t)
	f := newFakeJWKS(t, rsaJWK(kid, &key.PublicKey))
	v := newVerifier(f, "")
	now := time.Now()
	cases := []struct {
		name   string
		claims jwt.MapClaims
		accept bool
	}{
		{"AN-02a vencido hace 1 min", with(validClaims(), "exp", now.Add(-time.Minute).Unix()), false},
		{"AN-02b vencido hace 2 s (dentro del leeway)", with(validClaims(), "exp", now.Add(-2*time.Second).Unix()), true},
		{"AN-02c vencido hace 10 s (fuera del leeway)", with(validClaims(), "exp", now.Add(-10*time.Second).Unix()), false},
		{"AN-02d sin exp", with(validClaims(), "exp", nil), false},
		{"AN-02e otro emisor", with(validClaims(), "iss", "http://evil"), false},
		{"AN-02f sin emisor", with(validClaims(), "iss", nil), false},
		{"AN-02g otra audiencia", with(validClaims(), "aud", []string{"otra-api"}), false},
		{"AN-02h sin audiencia", with(validClaims(), "aud", nil), false},
		{"AN-02i audiencia como texto", with(validClaims(), "aud", audience), true},
		{"AN-02j audiencia entre varias", with(validClaims(), "aud", []string{"x", audience}), true},
		{"AN-02k emitido en el futuro", with(validClaims(), "iat", now.Add(time.Minute).Unix()), false},
		{"AN-02l nbf en el futuro", with(validClaims(), "nbf", now.Add(time.Minute).Unix()), false},
		{"AN-02m sub no es UUID", with(validClaims(), "sub", "admin"), false},
		{"AN-02n sin sub", with(validClaims(), "sub", nil), false},
		{"AN-02o sin sid (no se podría revocar)", with(validClaims(), "sid", nil), false},
		{"AN-02p sid vacío", with(validClaims(), "sid", ""), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := signRS(t, key, kid, c.claims)
			if c.accept {
				if _, err := v.Verify(context.Background(), raw); err != nil {
					t.Fatalf("rechazado: %v", err)
				}
				return
			}
			mustReject(t, v, raw)
		})
	}
}

// Caso AN-03 · firma, cabecera y algoritmo
// Técnica: Adivinación de errores (ataques conocidos a JWT).
// Entrada: firma con otra clave, sin kid, kid desconocido, payload alterado, alg none, HS256
// firmado con la clave pública RSA (confusión de algoritmo), RS512, PS256, ES256, basura y vacío
// Debe hacer / esperado: todos ErrInvalidToken.
func TestAN03_FirmaCabeceraYAlgoritmo(t *testing.T) {
	key, other, _ := keys(t)
	f := newFakeJWKS(t, rsaJWK(kid, &key.PublicKey))
	v := newVerifier(f, "")

	good := signRS(t, key, kid, validClaims())
	parts := strings.Split(good, ".")
	tampered := parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"`+issuer+`","aud":"`+audience+`","sub":"10000000-0000-0000-0000-000000000009","sid":"x","exp":9999999999}`)) + "." + parts[2]

	pubDER, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	confusion := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims())
	confusion.Header["kid"] = kid
	confusionRaw, _ := confusion.SignedString(pubDER)

	none := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)) + "." + strings.Split(good, ".")[1] + "."

	other512 := jwt.NewWithClaims(jwt.SigningMethodRS512, validClaims())
	other512.Header["kid"] = kid
	rs512, _ := other512.SignedString(key)
	ps := jwt.NewWithClaims(jwt.SigningMethodPS256, validClaims())
	ps.Header["kid"] = kid
	ps256, _ := ps.SignedString(key)
	ecKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	es := jwt.NewWithClaims(jwt.SigningMethodES256, validClaims())
	es.Header["kid"] = kid
	es256, _ := es.SignedString(ecKey)

	cases := map[string]string{
		"AN-03a firmado con otra clave":         signRS(t, other, kid, validClaims()),
		"AN-03b sin kid":                        signRS(t, key, "", validClaims()),
		"AN-03c kid desconocido":                signRS(t, key, "otro", validClaims()),
		"AN-03d payload alterado":               tampered,
		"AN-03e alg none":                       none,
		"AN-03f HS256 con la clave pública RSA": confusionRaw,
		"AN-03g RS512":                          rs512,
		"AN-03h PS256":                          ps256,
		"AN-03i ES256":                          es256,
		"AN-03j texto que no es JWT":            "abc",
		"AN-03k vacío":                          "",
		"AN-03l dos segmentos":                  parts[0] + "." + parts[1],
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) { mustReject(t, v, raw) })
	}
}

// Caso AN-04 · tokens HS256 de desarrollo
// Técnica: Tabla de decisión (secreto configurado x contenido del token).
// Entrada: tokens HS256 con user_id, sub o ambos; secreto incorrecto, vencidos, sin exp, con
// user_id que no es UUID; verificador con y sin DEV_HS256_SECRET
// Debe hacer / esperado: solo se aceptan con secreto configurado y token correcto; user_id
// tiene prioridad sobre sub; DevToken=true y sin SessionID; rol y conjunto del token se ignoran.
func TestAN04_TokensHS256DeDesarrollo(t *testing.T) {
	key, _, _ := keys(t)
	f := newFakeJWKS(t, rsaJWK(kid, &key.PublicKey))
	dev := newVerifier(f, devKey)
	prod := newVerifier(f, "")
	exp := time.Now().Add(time.Hour).Unix()
	other := "10000000-0000-0000-0000-000000000002"

	accept := map[string]struct {
		claims jwt.MapClaims
		want   string
	}{
		"AN-04a user_id":                    {jwt.MapClaims{"user_id": userID, "exp": exp, "role": "ADMIN", "complex_id": "x"}, userID},
		"AN-04b solo sub":                   {jwt.MapClaims{"sub": userID, "exp": exp}, userID},
		"AN-04c user_id gana a sub":         {jwt.MapClaims{"user_id": userID, "sub": other, "exp": exp}, userID},
		"AN-04d vencido dentro del leeway":  {jwt.MapClaims{"user_id": userID, "exp": time.Now().Add(-2 * time.Second).Unix()}, userID},
		"AN-04e sin iss ni aud (dev token)": {jwt.MapClaims{"user_id": userID, "exp": exp}, userID},
	}
	for name, c := range accept {
		t.Run(name, func(t *testing.T) {
			id, err := dev.Verify(context.Background(), signHS(t, devKey, c.claims))
			if err != nil || id != (Identity{UserID: c.want, DevToken: true}) {
				t.Fatalf("Verify = %+v, %v", id, err)
			}
		})
	}

	reject := map[string]string{
		"AN-04f otro secreto":       signHS(t, "otro-secreto", jwt.MapClaims{"user_id": userID, "exp": exp}),
		"AN-04g vencido":            signHS(t, devKey, jwt.MapClaims{"user_id": userID, "exp": time.Now().Add(-time.Minute).Unix()}),
		"AN-04h sin exp":            signHS(t, devKey, jwt.MapClaims{"user_id": userID}),
		"AN-04i user_id no es UUID": signHS(t, devKey, jwt.MapClaims{"user_id": "admin", "exp": exp}),
		"AN-04j sin usuario":        signHS(t, devKey, jwt.MapClaims{"exp": exp}),
	}
	for name, raw := range reject {
		t.Run(name, func(t *testing.T) { mustReject(t, dev, raw) })
	}
	t.Run("AN-04k sin DEV_HS256_SECRET se rechaza todo HS256", func(t *testing.T) {
		mustReject(t, prod, signHS(t, devKey, jwt.MapClaims{"user_id": userID, "exp": exp}))
		mustReject(t, prod, signHS(t, "", jwt.MapClaims{"user_id": userID, "exp": exp}))
	})
}

// Caso AN-05 · claves del JWKS que se descartan
// Técnica: Partición en clases de equivalencia (una clave inválida por clase) y valores límite
// del tamaño (1024 / 2048 bits) y del exponente.
// Entrada: JWKS con una clave inválida junto a la válida
// Debe hacer / esperado: la inválida no se carga (su kid no verifica nada) y la válida sí.
func TestAN05_ClavesDelJWKSQueSeDescartan(t *testing.T) {
	key, other, small := keys(t)
	good := rsaJWK(kid, &key.PublicKey)
	bad := func(mut func(jwk)) jwk {
		k := rsaJWK("bad", &other.PublicKey)
		mut(k)
		return k
	}
	cases := map[string]jwk{
		"AN-05a kty EC":         bad(func(k jwk) { k["kty"] = "EC" }),
		"AN-05b sin kid":        bad(func(k jwk) { k["kid"] = "" }),
		"AN-05c use enc":        bad(func(k jwk) { k["use"] = "enc" }),
		"AN-05d n no es base64": bad(func(k jwk) { k["n"] = "***" }),
		"AN-05e e no es base64": bad(func(k jwk) { k["e"] = "***" }),
		"AN-05f clave de 1024":  rsaJWK("bad", &small.PublicKey),
		"AN-05g exponente 1":    bad(func(k jwk) { k["e"] = base64.RawURLEncoding.EncodeToString([]byte{1}) }),
		"AN-05h exponente 2^31": bad(func(k jwk) { k["e"] = base64.RawURLEncoding.EncodeToString(big.NewInt(1 << 31).Bytes()) }),
		"AN-05i exponente 2^64": bad(func(k jwk) {
			k["e"] = base64.RawURLEncoding.EncodeToString(new(big.Int).Lsh(big.NewInt(1), 64).Bytes())
		}),
	}
	for name, k := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFakeJWKS(t, good, k)
			v := newVerifier(f, "")
			if err := v.Warm(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(v.jwks.keys) != 1 || v.jwks.keys[kid] == nil {
				t.Fatalf("claves cargadas: %v", v.jwks.keys)
			}
		})
	}
	t.Run("AN-05j use vacío se acepta", func(t *testing.T) {
		k := rsaJWK(kid, &key.PublicKey)
		delete(k, "use")
		v := newVerifier(newFakeJWKS(t, k), "")
		if err := v.Warm(context.Background()); err != nil || v.jwks.keys[kid] == nil {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("AN-05l exponente 2^31-1 (máximo) se acepta", func(t *testing.T) {
		k := rsaJWK(kid, &key.PublicKey)
		k["e"] = base64.RawURLEncoding.EncodeToString(big.NewInt(1<<31 - 1).Bytes())
		v := newVerifier(newFakeJWKS(t, k), "")
		if err := v.Warm(context.Background()); err != nil || v.jwks.keys[kid].E != 1<<31-1 {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("AN-05k exponente 3 (mínimo) se acepta", func(t *testing.T) {
		k := rsaJWK(kid, &key.PublicKey)
		k["e"] = base64.RawURLEncoding.EncodeToString([]byte{3})
		v := newVerifier(newFakeJWKS(t, k), "")
		if err := v.Warm(context.Background()); err != nil || v.jwks.keys[kid].E != 3 {
			t.Fatalf("err=%v", err)
		}
	})
}

// Caso AN-06 · JWKS inutilizable
// Técnica: Adivinación de errores.
// Entrada: JWKS que responde 500, JSON inválido, lista vacía, solo claves inválidas o está caído
// Debe hacer / esperado: Warm devuelve error, Ready sigue en false y un token válido se rechaza.
func TestAN06_JWKSInutilizable(t *testing.T) {
	key, _, small := keys(t)
	cases := map[string]func(f *fakeJWKS){
		"AN-06a status 500":        func(f *fakeJWKS) { f.respond(500, "") },
		"AN-06b JSON inválido":     func(f *fakeJWKS) { f.respond(200, `{"keys":`) },
		"AN-06c sin claves":        func(f *fakeJWKS) { f.serve() },
		"AN-06d solo claves malas": func(f *fakeJWKS) { f.serve(rsaJWK(kid, &small.PublicKey)) },
		"AN-06e servidor caído":    func(f *fakeJWKS) { f.Close() },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFakeJWKS(t)
			setup(f)
			v := newVerifier(f, "")
			if err := v.Warm(context.Background()); err == nil {
				t.Fatal("Warm sin error")
			}
			if v.Ready() {
				t.Fatal("Ready=true sin claves")
			}
			mustReject(t, v, signRS(t, key, kid, validClaims()))
		})
	}
	t.Run("AN-06f URL inválida", func(t *testing.T) {
		v := NewVerifier(Config{JWKSURL: "http://[::1", Issuer: issuer, Audience: audience})
		if err := v.Warm(context.Background()); err == nil {
			t.Fatal("Warm sin error")
		}
	})
}

// Caso AN-07 · carga perezosa y Ready
// Técnica: Transición de estados (sin claves -> cargadas).
// Entrada: verificador sin Warm; primer token válido
// Debe hacer / esperado: Ready=false antes; el primer Verify descarga el JWKS (1 llamada), pasa
// y Ready=true; el segundo Verify usa la caché.
func TestAN07_CargaPerezosaYReady(t *testing.T) {
	key, _, _ := keys(t)
	f := newFakeJWKS(t, rsaJWK(kid, &key.PublicKey))
	v := newVerifier(f, "")
	if v.Ready() {
		t.Fatal("Ready antes de cargar")
	}
	raw := signRS(t, key, kid, validClaims())
	for i := 0; i < 2; i++ {
		if _, err := v.Verify(context.Background(), raw); err != nil {
			t.Fatalf("Verify %d: %v", i, err)
		}
	}
	if !v.Ready() || f.calls.Load() != 1 {
		t.Fatalf("Ready=%v llamadas=%d", v.Ready(), f.calls.Load())
	}
}

// Caso AN-08 · rotación de claves y protección contra kids falsos
// Técnica: Transición de estados y valores límite del intervalo mínimo de 30 s.
// Entrada: JWKS cargado con k1; auth rota a k2; llegan tokens con k2 y con kids inventados
// Debe hacer / esperado: dentro de los 30 s desde el último intento no se vuelve a pedir el JWKS
// (un kid inventado no golpea a auth); pasado ese intervalo, un kid nuevo dispara una sola
// descarga y el token con k2 se acepta.
func TestAN08_RotacionDeClaves(t *testing.T) {
	key, other, _ := keys(t)
	f := newFakeJWKS(t, rsaJWK(kid, &key.PublicKey))
	v := newVerifier(f, "")
	if err := v.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.serve(rsaJWK(kid, &key.PublicKey), rsaJWK("k2", &other.PublicKey))
	rotated := signRS(t, other, "k2", validClaims())

	v.jwks.lastTry = time.Now() // un intento reciente
	for i := 0; i < 5; i++ {
		mustReject(t, v, signRS(t, key, "inventado", validClaims()))
	}
	mustReject(t, v, rotated)
	if n := f.calls.Load(); n != 1 {
		t.Fatalf("descargas dentro del intervalo: %d, esperaba 1", n)
	}

	v.jwks.lastTry = time.Now().Add(-jwksRefreshMinInterval)
	if _, err := v.Verify(context.Background(), rotated); err != nil {
		t.Fatalf("clave rotada rechazada: %v", err)
	}
	if n := f.calls.Load(); n != 2 {
		t.Fatalf("descargas: %d, esperaba 2", n)
	}
	// La clave anterior sigue publicada y sigue sirviendo.
	if _, err := v.Verify(context.Background(), signRS(t, key, kid, validClaims())); err != nil {
		t.Fatalf("clave anterior: %v", err)
	}
}

// Caso AN-09 · caché del JWKS vencida
// Técnica: Transición de estados (fresca -> vencida -> refrescada / auth caído).
// Entrada: claves cargadas hace más de 10 min; (a) auth responde; (b) auth caído; (c) auth
// publica otro juego sin la clave del token
// Debe hacer / esperado: (a) se refresca y se acepta; (b) se sigue usando la clave en caché;
// (c) la clave retirada deja de servir.
func TestAN09_CacheDelJWKSVencida(t *testing.T) {
	key, other, _ := keys(t)
	stale := func(v *Verifier) {
		v.jwks.fetchedAt = time.Now().Add(-jwksCacheTTL - time.Second)
		v.jwks.lastTry = time.Now().Add(-jwksRefreshMinInterval - time.Second)
	}
	raw := signRS(t, key, kid, validClaims())

	t.Run("AN-09a refresca", func(t *testing.T) {
		f := newFakeJWKS(t, rsaJWK(kid, &key.PublicKey))
		v := newVerifier(f, "")
		v.Warm(context.Background())
		stale(v)
		if _, err := v.Verify(context.Background(), raw); err != nil || f.calls.Load() != 2 {
			t.Fatalf("err=%v llamadas=%d", err, f.calls.Load())
		}
	})
	t.Run("AN-09b auth caído: usa la caché", func(t *testing.T) {
		f := newFakeJWKS(t, rsaJWK(kid, &key.PublicKey))
		v := newVerifier(f, "")
		v.Warm(context.Background())
		stale(v)
		f.respond(503, "")
		if _, err := v.Verify(context.Background(), raw); err != nil {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("AN-09c clave retirada", func(t *testing.T) {
		f := newFakeJWKS(t, rsaJWK(kid, &key.PublicKey))
		v := newVerifier(f, "")
		v.Warm(context.Background())
		stale(v)
		f.serve(rsaJWK("k2", &other.PublicKey))
		mustReject(t, v, raw)
	})
	t.Run("AN-09d vencida pero intento reciente: usa la caché sin descargar", func(t *testing.T) {
		f := newFakeJWKS(t, rsaJWK(kid, &key.PublicKey))
		v := newVerifier(f, "")
		v.Warm(context.Background())
		v.jwks.fetchedAt = time.Now().Add(-jwksCacheTTL - time.Second)
		v.jwks.lastTry = time.Now()
		if _, err := v.Verify(context.Background(), raw); err != nil || f.calls.Load() != 1 {
			t.Fatalf("err=%v llamadas=%d", err, f.calls.Load())
		}
	})
}

// Caso AN-10 · cliente HTTP por defecto
// Técnica: Partición (parámetro nil).
// Entrada: Config sin HTTPClient
// Debe hacer / esperado: cliente con timeout de 3 s, para que un auth colgado no bloquee el gateway.
func TestAN10_ClientePorDefecto(t *testing.T) {
	v := NewVerifier(Config{JWKSURL: "http://auth/jwks"})
	if v.cfg.HTTPClient == nil || v.cfg.HTTPClient.Timeout != 3*time.Second {
		t.Fatalf("client %+v", v.cfg.HTTPClient)
	}
}
