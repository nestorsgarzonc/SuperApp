package config

import (
	"strings"
	"testing"
	"time"
)

// Caso CF-04 · URLs de los servicios
// Técnica: Partición en clases de equivalencia (URL válida / inválida) por cada servicio.
// Entrada: AUTH_URL, CONTROL_ACCESOS_URL o NOTIFICACIONES_URL ausente, relativa, sin host, con
// esquema ftp o mal formada
// Debe hacer / esperado: Load falla nombrando la variable; http y https se aceptan.
func TestCF04_URLsDeLosServicios(t *testing.T) {
	for _, key := range []string{"AUTH_URL", "CONTROL_ACCESOS_URL", "NOTIFICACIONES_URL"} {
		for name, val := range map[string]string{"vacía": "", "relativa": "/auth", "sin host": "http://", "ftp": "ftp://auth:21", "mal formada": "http://[::1"} {
			t.Run("CF-04 "+key+" "+name, func(t *testing.T) {
				base(t)
				t.Setenv(key, val)
				if _, err := Load(); err == nil || !strings.Contains(err.Error(), key) {
					t.Fatalf("err = %v", err)
				}
			})
		}
		t.Run("CF-04 "+key+" https", func(t *testing.T) {
			base(t)
			t.Setenv(key, "https://svc.internal:443/base")
			if _, err := Load(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Caso CF-05 · secreto HMAC del tenant
// Técnica: Valores límite (vacío, 31 y 32 bytes).
// Entrada: AUTH_TENANT_HMAC_SECRET de distintos largos
// Debe hacer / esperado: vacío se permite (no se envía tenant), 31 bytes falla, 32 bytes pasa.
func TestCF05_SecretoHMACDelTenant(t *testing.T) {
	for _, c := range []struct {
		name string
		val  string
		ok   bool
	}{
		{"CF-05a vacío", "", true},
		{"CF-05b 31 bytes", strings.Repeat("k", 31), false},
		{"CF-05c 32 bytes", strings.Repeat("k", 32), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			base(t)
			t.Setenv("AUTH_TENANT_HMAC_SECRET", c.val)
			if _, err := Load(); (err == nil) != c.ok {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// Caso CF-06 · valores numéricos
// Técnica: Valores límite (0, 1, negativo) y adivinación de errores (texto no numérico).
// Entrada: cada variable numérica en 0, -1, "abc" y 1
// Debe hacer / esperado: 0, negativos y texto hacen fallar el arranque; 1 se acepta.
func TestCF06_ValoresNumericos(t *testing.T) {
	keys := []string{"RATE_LIMIT_PUBLIC_PER_MIN", "RATE_LIMIT_USER_PER_MIN", "MAX_BODY_BYTES", "UPSTREAM_TIMEOUT_SECONDS", "ROLE_CACHE_TTL_SECONDS"}
	for _, key := range keys {
		for _, val := range []string{"0", "-1", "abc", "1"} {
			t.Run("CF-06 "+key+"="+val, func(t *testing.T) {
				base(t)
				t.Setenv(key, val)
				_, err := Load()
				if (val == "1") != (err == nil) {
					t.Fatalf("err = %v", err)
				}
			})
		}
	}
}

// Caso CF-07 · valores por defecto y explícitos
// Técnica: Partición.
// Entrada: entorno mínimo y entorno con todas las opciones
// Debe hacer / esperado: los defaults documentados en el README; los valores explícitos se
// respetan (orígenes sin barra final y sin vacíos, JWKS explícito, booleans).
func TestCF07_ValoresPorDefectoYExplicitos(t *testing.T) {
	t.Run("CF-07a defaults", func(t *testing.T) {
		base(t)
		c, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if c.Port != "8080" || c.AuthAudience != "superapp-api" || c.PublicRatePerMin != 10 || c.MaxBodyBytes != 1<<20 ||
			c.UpstreamTimeout != 30*time.Second || c.RoleCacheTTL != time.Minute || c.TrustForwardedFor || c.ExposeDevRoutes ||
			c.IsDevelopment() || len(c.CORSAllowedOrigins) != 0 || c.ControlDeAccesos.Secret != "a" || c.Auth.Secret != "" {
			t.Fatalf("%+v", c)
		}
	})
	t.Run("CF-07b explícitos", func(t *testing.T) {
		base(t)
		t.Setenv("ENV", "development")
		t.Setenv("PORT", "9000")
		t.Setenv("AUTH_JWKS_URL", "http://keys/jwks")
		t.Setenv("CORS_ALLOWED_ORIGINS", " http://a.example/ , ,http://b.example")
		t.Setenv("TRUST_FORWARDED_FOR", "true")
		t.Setenv("EXPOSE_DEV_ROUTES", "1")
		t.Setenv("UPSTREAM_TIMEOUT_SECONDS", "5")
		c, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if c.Port != "9000" || c.AuthJWKSURL != "http://keys/jwks" || strings.Join(c.CORSAllowedOrigins, "|") != "http://a.example|http://b.example" ||
			!c.TrustForwardedFor || !c.ExposeDevRoutes || c.UpstreamTimeout != 5*time.Second || !c.IsDevelopment() {
			t.Fatalf("%+v", c)
		}
	})
	t.Run("CF-07c booleano inválido es false", func(t *testing.T) {
		base(t)
		t.Setenv("TRUST_FORWARDED_FOR", "si")
		if c, _ := Load(); c.TrustForwardedFor {
			t.Fatal("TRUST_FORWARDED_FOR=si se tomó como true")
		}
	})
}

// Caso CF-08 · cualquier ENV distinto de development es producción
// Técnica: Partición en clases de equivalencia y adivinación de errores (mayúsculas, typos).
// Entrada: ENV=staging, Development, dev y prod con DEV_HS256_SECRET definido
// Debe hacer / esperado: todos fallan; solo "development" exacto habilita las opciones de desarrollo.
func TestCF08_CualquierENVDistintoEsProduccion(t *testing.T) {
	for _, env := range []string{"staging", "Development", "dev", "prod"} {
		t.Run("CF-08 "+env, func(t *testing.T) {
			base(t)
			t.Setenv("ENV", env)
			t.Setenv("DEV_HS256_SECRET", "x")
			if _, err := Load(); err == nil {
				t.Fatal("aceptó DEV_HS256_SECRET")
			}
		})
	}
}

// Caso CF-09 · se informan todos los errores juntos
// Técnica: Adivinación de errores.
// Entrada: entorno vacío
// Debe hacer / esperado: un solo error que nombra AUTH_URL, CONTROL_ACCESOS_URL,
// NOTIFICACIONES_URL, AUTH_JWT_ISSUER e INTERNAL_SECRET.
func TestCF09_TodosLosErroresJuntos(t *testing.T) {
	for _, k := range []string{"AUTH_URL", "CONTROL_ACCESOS_URL", "NOTIFICACIONES_URL", "AUTH_JWT_ISSUER", "INTERNAL_SECRET"} {
		t.Setenv(k, "")
	}
	_, err := Load()
	for _, want := range []string{"AUTH_URL", "CONTROL_ACCESOS_URL", "NOTIFICACIONES_URL", "AUTH_JWT_ISSUER", "INTERNAL_SECRET"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("CF-09 falta %s en %v", want, err)
		}
	}
}
