package apierr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Caso AE-01 · sobre de error común
// Técnica: Partición en clases de equivalencia (un caso por error del catálogo).
// Entrada: cada Error predefinido escrito con Write
// Debe hacer / esperado: el status del error, Content-Type JSON, Cache-Control no-store y el
// cuerpo {"success":false,"error":{"code","message"}} sin otros campos.
func TestAE01_SobreDeErrorComun(t *testing.T) {
	for _, e := range []Error{Unauthenticated, NotMember, NotFound, PayloadTooLarge, BadComplexID, RateLimited, Unavailable, UpstreamTimeout} {
		t.Run("AE-01 "+e.Code+" "+http.StatusText(e.Status), func(t *testing.T) {
			rec := httptest.NewRecorder()
			Write(rec, e)
			if rec.Code != e.Status {
				t.Fatalf("status %d, esperaba %d", rec.Code, e.Status)
			}
			if rec.Header().Get("Content-Type") != "application/json" || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("headers %v", rec.Header())
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			inner, _ := body["error"].(map[string]any)
			if len(body) != 2 || body["success"] != false || inner["code"] != e.Code || inner["message"] != e.Message || len(inner) != 2 {
				t.Fatalf("cuerpo %s", rec.Body.String())
			}
		})
	}
}

// Caso AE-02 · Retry-After en segundos enteros
// Técnica: Valores límite.
// Entrada: retryAfter de 0, 1 ns, 499 ms, 1,4 s, 1,5 s, 59,6 s y negativo
// Debe hacer / esperado: 429 SYS_004 con Retry-After redondeado al segundo y nunca menor que 1.
func TestAE02_RetryAfterEnSegundosEnteros(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"AE-02a cero", 0, "1"},
		{"AE-02b un nanosegundo", time.Nanosecond, "1"},
		{"AE-02c 499ms redondea a 0 y sube a 1", 499 * time.Millisecond, "1"},
		{"AE-02d 1,4s", 1400 * time.Millisecond, "1"},
		{"AE-02e 1,5s redondea hacia arriba", 1500 * time.Millisecond, "2"},
		{"AE-02f 59,6s", 59600 * time.Millisecond, "60"},
		{"AE-02g negativo", -time.Second, "1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteRetryAfter(rec, c.in)
			if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != c.want {
				t.Fatalf("status %d Retry-After %q, esperaba 429 %q", rec.Code, rec.Header().Get("Retry-After"), c.want)
			}
			var body struct{ Error struct{ Code string } }
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if body.Error.Code != "SYS_004" {
				t.Fatalf("code %q", body.Error.Code)
			}
		})
	}
}
