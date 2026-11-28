package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func corsRequest(method, origin, requestMethod string) *http.Request {
	r := httptest.NewRequest(method, "/api/v1/access", nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if requestMethod != "" {
		r.Header.Set("Access-Control-Request-Method", requestMethod)
	}
	return r
}

// Caso CR-01 · política CORS
// Técnica: Tabla de decisión (origen permitido x preflight x comodín).
// Entrada: combinaciones de Origin, método OPTIONS y Access-Control-Request-Method contra la
// lista ["http://localhost:3000"] o ["*"]
// Debe hacer / esperado: solo un origen permitido recibe Allow-Origin; solo un preflight se
// contesta aquí (204, con Allow-* si el origen es permitido); nunca se permiten credenciales.
func TestCR01_PoliticaCORS(t *testing.T) {
	const allowed = "http://localhost:3000"
	cases := []struct {
		name             string
		list             []string
		method, origin   string
		reqMethod        string
		answered         bool
		allowOrigin      string
		allowMethodsSent bool
	}{
		{"CR-01a sin Origin", []string{allowed}, "GET", "", "", false, "", false},
		{"CR-01b sin Origin aunque sea OPTIONS", []string{allowed}, "OPTIONS", "", "POST", false, "", false},
		{"CR-01c origen permitido, petición simple", []string{allowed}, "GET", allowed, "", false, allowed, false},
		{"CR-01d origen permitido con barra final", []string{allowed}, "GET", allowed + "/", "", false, allowed + "/", false},
		{"CR-01e origen no permitido", []string{allowed}, "GET", "https://evil.example", "", false, "", false},
		{"CR-01f subdominio no permitido", []string{allowed}, "GET", "http://localhost:3000.evil.example", "", false, "", false},
		{"CR-01g preflight permitido", []string{allowed}, "OPTIONS", allowed, "POST", true, allowed, true},
		{"CR-01h preflight no permitido", []string{allowed}, "OPTIONS", "https://evil.example", "POST", true, "", false},
		{"CR-01i OPTIONS sin Request-Method no es preflight", []string{allowed}, "OPTIONS", allowed, "", false, allowed, false},
		{"CR-01j comodín", []string{"*"}, "GET", "https://cualquiera.example", "", false, "*", false},
		{"CR-01k comodín en preflight", []string{"*"}, "OPTIONS", "https://cualquiera.example", "DELETE", true, "*", true},
		{"CR-01l lista vacía", nil, "OPTIONS", allowed, "POST", true, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			answered := newCORS(c.list).apply(rec, corsRequest(c.method, c.origin, c.reqMethod))
			h := rec.Header()
			if answered != c.answered {
				t.Fatalf("answered = %v", answered)
			}
			if answered && rec.Code != http.StatusNoContent {
				t.Fatalf("status %d", rec.Code)
			}
			if got := h.Get("Access-Control-Allow-Origin"); got != c.allowOrigin {
				t.Fatalf("Allow-Origin = %q", got)
			}
			if (h.Get("Access-Control-Allow-Methods") != "") != c.allowMethodsSent {
				t.Fatalf("Allow-Methods = %q", h.Get("Access-Control-Allow-Methods"))
			}
			if c.allowMethodsSent && (h.Get("Access-Control-Allow-Headers") != corsHeaders || h.Get("Access-Control-Max-Age") != "600") {
				t.Fatalf("preflight headers %v", h)
			}
			if c.allowOrigin != "" && h.Get("Access-Control-Expose-Headers") != "X-Request-ID, Retry-After" {
				t.Fatalf("Expose-Headers = %q", h.Get("Access-Control-Expose-Headers"))
			}
			if c.origin != "" && h.Get("Vary") != "Origin" {
				t.Fatalf("Vary = %q (las cachés deben separar por origen)", h.Get("Vary"))
			}
			if h.Get("Access-Control-Allow-Credentials") != "" {
				t.Fatal("nunca se permiten credenciales")
			}
		})
	}
}

// Caso CR-02 · cabeceras permitidas en el preflight
// Técnica: Partición.
// Entrada: preflight de un origen permitido
// Debe hacer / esperado: se permiten Authorization, Content-Type, X-Complex-ID y X-Request-ID y
// los métodos GET, POST, PUT, PATCH, DELETE y OPTIONS; X-User-ID no se ofrece.
func TestCR02_CabecerasPermitidas(t *testing.T) {
	rec := httptest.NewRecorder()
	newCORS([]string{"http://localhost:3000"}).apply(rec, corsRequest("OPTIONS", "http://localhost:3000", "POST"))
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type, X-Complex-ID, X-Request-ID" {
		t.Fatalf("Allow-Headers = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, PATCH, DELETE, OPTIONS" {
		t.Fatalf("Allow-Methods = %q", got)
	}
}
