package gateway

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"superapp/api-gateway/internal/config"
)

func routesConfig(dev bool) config.Config {
	return config.Config{
		Auth:             config.Upstream{URL: mustURL("http://auth:8081")},
		ControlDeAccesos: config.Upstream{URL: mustURL("http://ca:8080"), Secret: caSecret},
		Notificaciones:   config.Upstream{URL: mustURL("http://notif:8083"), Secret: notifSecret},
		ExposeDevRoutes:  dev,
	}
}

// Caso RT-01 · paths peligrosos
// Técnica: Partición en clases de equivalencia y adivinación de errores (path traversal).
// Entrada: path y RawPath como los entrega net/http
// Debe hacer / esperado: solo los paths limpios son seguros; segmentos . y .., //, \, NUL y
// separadores o puntos codificados (%2f %5c %2e %00, en cualquier caja) se rechazan.
func TestRT01_PathsPeligrosos(t *testing.T) {
	cases := []struct {
		name, path, raw string
		safe            bool
	}{
		{"RT-01a path normal", "/api/v1/access/qr-codes", "", true},
		{"RT-01b raíz", "/", "", true},
		{"RT-01c punto dentro de un segmento", "/api/v1/files/a.b", "", true},
		{"RT-01d tres puntos", "/api/v1/.../x", "", true},
		{"RT-01e espacio codificado", "/api/v1/a b", "/api/v1/a%20b", true},
		{"RT-01f sin barra inicial", "api/v1/access", "", false},
		{"RT-01g vacío", "", "", false},
		{"RT-01h segmento ..", "/api/v1/notifications/../internal", "", false},
		{"RT-01i segmento .", "/api/v1/./access", "", false},
		{"RT-01j .. al final", "/api/v1/access/..", "", false},
		{"RT-01k doble barra", "/api/v1//access", "", false},
		{"RT-01l barra invertida", "/api/v1/access\\..\\internal", "", false},
		{"RT-01m byte NUL", "/api/v1/access\x00", "", false},
		{"RT-01n %2f", "/api/v1/a/b", "/api/v1/a%2fb", false},
		{"RT-01o %2F mayúscula", "/api/v1/a/b", "/api/v1/a%2Fb", false},
		{"RT-01p %5c", "/api/v1/a\\b", "/api/v1/a%5cb", false},
		{"RT-01q %2e%2e", "/api/v1/../x", "/api/v1/%2e%2e/x", false},
		{"RT-01r %2E", "/api/v1/.x", "/api/v1/%2Ex", false},
		{"RT-01s %00", "/api/v1/a", "/api/v1/a%00", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := safePath(c.path, c.raw); got != c.safe {
				t.Fatalf("safePath(%q, %q) = %v", c.path, c.raw, got)
			}
		})
	}
}

// Caso RT-02 · rutas internas
// Técnica: Partición en clases de equivalencia.
// Entrada: paths ya traducidos al microservicio
// Debe hacer / esperado: cualquier segmento "internal" (sin importar mayúsculas) es interno; un
// segmento que solo lo contiene no.
func TestRT02_RutasInternas(t *testing.T) {
	cases := map[string]bool{
		"/internal/users":               true,
		"/api/v1/internal":              true,
		"/notifications/INTERNAL/x":     true,
		"/Internal":                     true,
		"/api/v1/internals":             false,
		"/api/v1/access/internal-notes": false,
		"/api/v1/access":                false,
		"/":                             false,
	}
	for path, want := range cases {
		t.Run("RT-02 "+path, func(t *testing.T) {
			if got := internalPath(path); got != want {
				t.Fatalf("internalPath(%q) = %v", path, got)
			}
		})
	}
}

// Caso RT-03 · tabla de rutas
// Técnica: Tabla de decisión (path -> servicio, firma de tenant) y valores límite del prefijo
// (prefijo exacto, prefijo + "/", prefijo + letra).
// Entrada: paths públicos del front
// Debe hacer / esperado: cada path va al servicio correcto o no tiene ruta; solo las rutas de
// auth firman el tenant.
func TestRT03_TablaDeRutas(t *testing.T) {
	routes := BuildRoutes(routesConfig(false))
	cases := []struct {
		path, service string
		signs         bool
	}{
		{"/api/v1/auth/login", "auth", true},
		{"/api/v1/auth", "auth", true},
		{"/api/v1/users/me", "auth", true},
		{"/api/v1/roles", "auth", true},
		{"/api/v1/permissions/x", "auth", true},
		{"/api/v1/sessions", "auth", true},
		{"/api/v1/notifications/users/me", "notificaciones", false},
		{"/api/v1/notifications", "notificaciones", false},
		{"/api/v1/access/validate", "control-de-accesos", false},
		{"/api/v1/access", "control-de-accesos", false},
		{"/api/v1/access-grants/1", "control-de-accesos", false},
		{"/api/v1/access-logs", "control-de-accesos", false},
		{"/api/v1/resident-movements", "control-de-accesos", false},
		{"/api/v1/units/x/access-grants", "control-de-accesos", false},
		{"/api/v1/me/memberships", "control-de-accesos", false},
		{"/api/v1/accessories", "", false},
		{"/api/v1/authx", "", false},
		{"/api/v1/memberships", "", false},
		{"/api/v1/dev/users", "", false},
		{"/api/v1", "", false},
		{"/", "", false},
		{"/API/V1/AUTH/LOGIN", "", false},
	}
	for _, c := range cases {
		t.Run("RT-03 "+c.path, func(t *testing.T) {
			r, ok := match(routes, c.path)
			if c.service == "" {
				if ok {
					t.Fatalf("enrutado a %s", r.Name)
				}
				return
			}
			if !ok || r.Name != c.service || r.SignTenant != c.signs {
				t.Fatalf("ok=%v servicio=%q firma=%v", ok, r.Name, r.SignTenant)
			}
		})
	}
}

// Caso RT-04 · prefijo de notificaciones
// Técnica: Valores límite (prefijo exacto, con barra y con subruta).
// Entrada: paths bajo /api/v1/notifications y una ruta sin Strip
// Debe hacer / esperado: el prefijo se quita y nunca queda un path vacío; sin Strip el path
// llega igual.
func TestRT04_PrefijoDeNotificaciones(t *testing.T) {
	routes := BuildRoutes(routesConfig(false))
	notif, _ := match(routes, "/api/v1/notifications")
	ca, _ := match(routes, "/api/v1/access")
	cases := []struct {
		name  string
		route Route
		in    string
		want  string
	}{
		{"RT-04a prefijo exacto", notif, "/api/v1/notifications", "/"},
		{"RT-04b prefijo con barra", notif, "/api/v1/notifications/", "/"},
		{"RT-04c subruta", notif, "/api/v1/notifications/users/me/notifications", "/users/me/notifications"},
		{"RT-04d sin Strip", ca, "/api/v1/access/validate", "/api/v1/access/validate"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.route.upstreamPath(c.in); got != c.want {
				t.Fatalf("upstreamPath(%q) = %q", c.in, got)
			}
		})
	}
}

// Caso RT-05 · rutas públicas de auth
// Técnica: Tabla de decisión (método x path).
// Entrada: combinaciones de método y path bajo /api/v1/auth
// Debe hacer / esperado: solo POST a login, refresh, logout, register, forgot-password y
// reset-password (con o sin barra final) es público; todo lo demás exige token.
func TestRT05_RutasPublicasDeAuth(t *testing.T) {
	auth, _ := match(BuildRoutes(routesConfig(false)), "/api/v1/auth")
	cases := []struct {
		method, path string
		public       bool
	}{
		{"POST", "/api/v1/auth/login", true},
		{"POST", "/api/v1/auth/login/", true},
		{"POST", "/api/v1/auth/refresh", true},
		{"POST", "/api/v1/auth/logout", true},
		{"POST", "/api/v1/auth/register", true},
		{"POST", "/api/v1/auth/forgot-password", true},
		{"POST", "/api/v1/auth/reset-password", true},
		{"GET", "/api/v1/auth/login", false},
		{"PUT", "/api/v1/auth/login", false},
		{"OPTIONS", "/api/v1/auth/login", false},
		{"POST", "/api/v1/auth/logout-all", false},
		{"POST", "/api/v1/auth/me", false},
		{"POST", "/api/v1/auth", false},
		{"POST", "/api/v1/auth/login/extra", false},
	}
	for _, c := range cases {
		t.Run("RT-05 "+c.method+" "+c.path, func(t *testing.T) {
			if got := auth.Public(c.method, c.path); got != c.public {
				t.Fatalf("Public = %v", got)
			}
		})
	}
	for _, p := range []string{"/api/v1/users", "/api/v1/roles", "/api/v1/sessions"} {
		if r, _ := match(BuildRoutes(routesConfig(false)), p); r.Public != nil {
			t.Fatalf("RT-05 %s no debe tener rutas públicas", p)
		}
	}
}

// Caso RT-06 · rutas de desarrollo
// Técnica: Tabla de decisión (EXPOSE_DEV_ROUTES x método x path).
// Entrada: tabla de rutas con y sin rutas de desarrollo
// Debe hacer / esperado: sin la opción /api/v1/dev no existe; con ella, solo GET /dev/users y
// GET /dev/token son públicos.
func TestRT06_RutasDeDesarrollo(t *testing.T) {
	if _, ok := match(BuildRoutes(routesConfig(false)), "/api/v1/dev/users"); ok {
		t.Fatal("RT-06 rutas dev expuestas sin EXPOSE_DEV_ROUTES")
	}
	dev, ok := match(BuildRoutes(routesConfig(true)), "/api/v1/dev/users")
	if !ok || dev.Name != "control-de-accesos" {
		t.Fatal("RT-06 rutas dev no enrutadas")
	}
	cases := []struct {
		method, path string
		public       bool
	}{
		{"GET", "/api/v1/dev/users", true},
		{"GET", "/api/v1/dev/token", true},
		{"GET", "/api/v1/dev/token/", true},
		{"POST", "/api/v1/dev/token", false},
		{"GET", "/api/v1/dev/reset", false},
		{"GET", "/api/v1/dev", false},
	}
	for _, c := range cases {
		t.Run("RT-06 "+c.method+" "+c.path, func(t *testing.T) {
			if got := dev.Public(c.method, c.path); got != c.public {
				t.Fatalf("Public = %v", got)
			}
		})
	}
}

// Caso RT-07 · IP y protocolo del cliente
// Técnica: Tabla de decisión (TRUST_FORWARDED_FOR x cabeceras) y adivinación de errores
// (suplantación de IP).
// Entrada: RemoteAddr, X-Forwarded-For, X-Forwarded-Proto y TLS
// Debe hacer / esperado: sin confianza se usa la IP real de la conexión; con confianza, la última
// entrada válida de X-Forwarded-For; un protocolo distinto de http/https se ignora.
func TestRT07_IPYProtocoloDelCliente(t *testing.T) {
	cases := []struct {
		name, remote, xff, xfp string
		trust, tls             bool
		ip, proto              string
	}{
		{"RT-07a sin confianza ignora XFF", "10.0.0.5:4000", "1.1.1.1", "https", false, false, "10.0.0.5", "http"},
		{"RT-07b con confianza toma la última", "10.0.0.5:4000", "6.6.6.6, 1.1.1.1", "https", true, false, "1.1.1.1", "https"},
		{"RT-07c última con espacios", "10.0.0.5:4000", "6.6.6.6,   2.2.2.2  ", "", true, false, "2.2.2.2", "http"},
		{"RT-07d última no es IP", "10.0.0.5:4000", "1.1.1.1, evil", "", true, false, "10.0.0.5", "http"},
		{"RT-07e sin XFF", "10.0.0.5:4000", "", "", true, false, "10.0.0.5", "http"},
		{"RT-07f IPv6", "[2001:db8::1]:4000", "", "", false, false, "2001:db8::1", "http"},
		{"RT-07g XFF IPv6", "10.0.0.5:4000", "2001:db8::2", "", true, false, "2001:db8::2", "http"},
		{"RT-07h RemoteAddr sin puerto", "10.0.0.5", "", "", false, false, "10.0.0.5", "http"},
		{"RT-07i protocolo inválido", "10.0.0.5:4000", "", "gopher", true, false, "10.0.0.5", "http"},
		{"RT-07j TLS directo", "10.0.0.5:4000", "", "http", false, true, "10.0.0.5", "https"},
		{"RT-07k XFP con confianza gana a TLS", "10.0.0.5:4000", "", "http", true, true, "10.0.0.5", "http"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = c.remote
			if c.xff != "" {
				r.Header.Set("X-Forwarded-For", c.xff)
			}
			if c.xfp != "" {
				r.Header.Set("X-Forwarded-Proto", c.xfp)
			}
			if c.tls {
				r.TLS = &tls.ConnectionState{}
			}
			if ip := clientIP(r, c.trust); ip != c.ip {
				t.Fatalf("clientIP = %q", ip)
			}
			if p := forwardedProto(r, c.trust); p != c.proto {
				t.Fatalf("forwardedProto = %q", p)
			}
		})
	}
}

// Caso RT-08 · firma del tenant para auth
// Técnica: Partición (vector conocido) y adivinación de errores (cambiar un solo campo).
// Entrada: secreto, usuario, conjunto y timestamp fijos
// Debe hacer / esperado: el HMAC-SHA256 hexadecimal de "usuario\nconjunto\nts" calculado aparte;
// cambiar cualquier campo cambia la firma.
func TestRT08_FirmaDelTenant(t *testing.T) {
	// printf 'u\nt\n1700000000' | openssl dgst -sha256 -hmac 0123456789abcdef0123456789abcdef
	const want = "235c3d47d0f16f21f4f924ef01328397a1691e37eec49c5897fbd1ff795e94e2"
	got := SignTenant(tenantKey, "u", "t", "1700000000")
	if len(got) != 64 {
		t.Fatalf("firma %q", got)
	}
	if got != want {
		t.Fatalf("firma %q, esperaba %q", got, want)
	}
	for name, other := range map[string]string{
		"usuario":   SignTenant(tenantKey, "v", "t", "1700000000"),
		"conjunto":  SignTenant(tenantKey, "u", "s", "1700000000"),
		"timestamp": SignTenant(tenantKey, "u", "t", "1700000001"),
		"secreto":   SignTenant(tenantKey+"x", "u", "t", "1700000000"),
		"separador": SignTenant(tenantKey, "u\nt", "", "1700000000"),
	} {
		if other == got {
			t.Errorf("RT-08 cambiar %s no cambia la firma", name)
		}
	}
}
