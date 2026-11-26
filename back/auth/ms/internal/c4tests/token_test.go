package c4tests

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	authjwt "superapp/auth/internal/infrastructure/security/jwt"
)

const (
	tkIssuer   = "https://auth.example.test"
	tkAudience = "superapp-api"
)

// newKey genera una llave RSA de `bits` bits y la guarda como PEM en un archivo temporal.
func newKey(t *testing.T, bits int) (string, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "private.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err = os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, key
}

func loadTokens(t *testing.T, path, issuer, audience string, ttl time.Duration) *authjwt.Tokens {
	t.Helper()
	tokens, err := authjwt.LoadTokens(path, issuer, audience, ttl)
	if err != nil {
		t.Fatal(err)
	}
	return tokens
}

// kidOf lee el kid publicado en el JWKS (sin depender de tipos internos).
func kidOf(t *testing.T, tokens *authjwt.Tokens) string {
	t.Helper()
	raw, err := json.Marshal(tokens.JWKS())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Keys []struct {
			Kid string `json:"kid"`
			Alg string `json:"alg"`
		} `json:"keys"`
	}
	if err = json.Unmarshal(raw, &doc); err != nil || len(doc.Keys) != 1 || doc.Keys[0].Kid == "" {
		t.Fatalf("JWKS invalido: %s (%v)", raw, err)
	}
	return doc.Keys[0].Kid
}

func validClaims(sid string) gojwt.MapClaims {
	now := time.Now()
	return gojwt.MapClaims{
		"iss": tkIssuer, "sub": "user-1", "aud": []string{tkAudience},
		"iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(time.Minute).Unix(),
		"sid": sid, "jti": sid,
	}
}

// forge firma un token a mano con el metodo, la llave y el kid indicados.
func forge(t *testing.T, method gojwt.SigningMethod, claims gojwt.MapClaims, kid string, key any) string {
	t.Helper()
	tok := gojwt.NewWithClaims(method, claims)
	if kid != "" {
		tok.Header["kid"] = kid
	}
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// ===================== TOKENS DE ACCESO (TK-01 a TK-11) =====================

func TestTK01_TokenEmitidoLlevaLosClaimsEsperados(t *testing.T) {
	path, _ := newKey(t, 2048)
	tokens := loadTokens(t, path, tkIssuer, tkAudience, 15*time.Minute)
	raw, expires, err := tokens.Issue("user-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := tokens.Verify(raw)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "user-1" || claims.SessionID != "session-1" {
		t.Fatalf("claims inesperados: %#v", claims)
	}
	if d := time.Until(expires); d < 14*time.Minute || d > 15*time.Minute+time.Second {
		t.Fatalf("expira en %v, se esperaban 15 minutos", d)
	}
	parsed, _, err := gojwt.NewParser().ParseUnverified(raw, gojwt.MapClaims{})
	if err != nil {
		t.Fatal(err)
	}
	m := parsed.Claims.(gojwt.MapClaims)
	for _, k := range []string{"iss", "sub", "aud", "iat", "nbf", "exp", "sid", "jti"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("falta el claim %q", k)
		}
	}
	if m["iss"] != tkIssuer || parsed.Method.Alg() != "RS256" {
		t.Fatalf("emisor=%v algoritmo=%s", m["iss"], parsed.Method.Alg())
	}
}

func TestTK02_TokenVencidoSeRechaza(t *testing.T) {
	path, _ := newKey(t, 2048)
	tokens := loadTokens(t, path, tkIssuer, tkAudience, -time.Minute)
	raw, _, err := tokens.Issue("user-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tokens.Verify(raw); err == nil {
		t.Fatal("un token vencido fue aceptado")
	}
}

func TestTK03_TokenConCargaAlteradaSeRechaza(t *testing.T) {
	path, _ := newKey(t, 2048)
	tokens := loadTokens(t, path, tkIssuer, tkAudience, time.Minute)
	raw, _, err := tokens.Issue("user-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(raw, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err = json.Unmarshal(payload, &m); err != nil {
		t.Fatal(err)
	}
	m["sub"] = "atacante"
	forgedPayload, _ := json.Marshal(m)
	parts[1] = base64.RawURLEncoding.EncodeToString(forgedPayload)
	if _, err = tokens.Verify(strings.Join(parts, ".")); err == nil {
		t.Fatal("un token con la carga alterada fue aceptado")
	}
}

func TestTK04_TokenFirmadoConHS256SeRechaza(t *testing.T) {
	path, _ := newKey(t, 2048)
	tokens := loadTokens(t, path, tkIssuer, tkAudience, time.Minute)
	raw := forge(t, gojwt.SigningMethodHS256, validClaims("s1"), kidOf(t, tokens), []byte("clave-simetrica-de-prueba-0123456789"))
	if _, err := tokens.Verify(raw); err == nil {
		t.Fatal("un token HS256 fue aceptado")
	}
}

func TestTK05_TokenSinFirmaAlgNoneSeRechaza(t *testing.T) {
	path, _ := newKey(t, 2048)
	tokens := loadTokens(t, path, tkIssuer, tkAudience, time.Minute)
	raw := forge(t, gojwt.SigningMethodNone, validClaims("s1"), kidOf(t, tokens), gojwt.UnsafeAllowNoneSignatureType)
	if _, err := tokens.Verify(raw); err == nil {
		t.Fatal("un token con alg none fue aceptado")
	}
}

func TestTK06_TokenConKidDesconocidoSeRechaza(t *testing.T) {
	path, key := newKey(t, 2048)
	tokens := loadTokens(t, path, tkIssuer, tkAudience, time.Minute)
	raw := forge(t, gojwt.SigningMethodRS256, validClaims("s1"), "otra-llave", key)
	if _, err := tokens.Verify(raw); err == nil {
		t.Fatal("un token con kid desconocido fue aceptado")
	}
	// control: el mismo token con el kid correcto si debe aceptarse
	ok := forge(t, gojwt.SigningMethodRS256, validClaims("s1"), kidOf(t, tokens), key)
	if _, err := tokens.Verify(ok); err != nil {
		t.Fatalf("el token de control fue rechazado: %v", err)
	}
}

func TestTK07_TokenFirmadoConOtraLlaveSeRechaza(t *testing.T) {
	pathA, _ := newKey(t, 2048)
	pathB, _ := newKey(t, 2048)
	a := loadTokens(t, pathA, tkIssuer, tkAudience, time.Minute)
	b := loadTokens(t, pathB, tkIssuer, tkAudience, time.Minute)
	raw, _, err := b.Issue("user-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Verify(raw); err == nil {
		t.Fatal("un token firmado con otra llave fue aceptado")
	}
}

func TestTK08_TokenDeOtroEmisorSeRechaza(t *testing.T) {
	path, _ := newKey(t, 2048)
	a := loadTokens(t, path, tkIssuer, tkAudience, time.Minute)
	other := loadTokens(t, path, "https://otro-emisor.example.test", tkAudience, time.Minute)
	raw, _, err := other.Issue("user-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Verify(raw); err == nil {
		t.Fatal("un token de otro emisor fue aceptado")
	}
}

func TestTK09_TokenDeOtraAudienciaSeRechaza(t *testing.T) {
	path, _ := newKey(t, 2048)
	a := loadTokens(t, path, tkIssuer, tkAudience, time.Minute)
	other := loadTokens(t, path, tkIssuer, "otra-api", time.Minute)
	raw, _, err := other.Issue("user-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Verify(raw); err == nil {
		t.Fatal("un token de otra audiencia fue aceptado")
	}
}

func TestTK10_TokenSinIdDeSesionSeRechaza(t *testing.T) {
	path, key := newKey(t, 2048)
	tokens := loadTokens(t, path, tkIssuer, tkAudience, time.Minute)
	raw := forge(t, gojwt.SigningMethodRS256, validClaims(""), kidOf(t, tokens), key)
	if _, err := tokens.Verify(raw); err == nil {
		t.Fatal("un token sin sid fue aceptado")
	}
}

func TestTK11_LlaveDeMenosDe2048BitsNoSeCarga(t *testing.T) {
	path, _ := newKey(t, 1024)
	if _, err := authjwt.LoadTokens(path, tkIssuer, tkAudience, time.Minute); err == nil {
		t.Fatal("se cargo una llave RSA de 1024 bits")
	}
}
