package config

import (
	"strings"
	"testing"
)

func base(t *testing.T) {
	t.Setenv("AUTH_URL", "http://auth:8081")
	t.Setenv("CONTROL_ACCESOS_URL", "http://ca:8080")
	t.Setenv("CONTROL_ACCESOS_SECRET", "a")
	t.Setenv("NOTIFICACIONES_URL", "http://notif:8083")
	t.Setenv("NOTIFICACIONES_SECRET", "b")
	t.Setenv("AUTH_JWT_ISSUER", "http://auth:8081")
	t.Setenv("INTERNAL_SECRET", "c")
}

func TestCF01_LoadDefaults(t *testing.T) {
	base(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Env != "production" || c.AuthJWKSURL != "http://auth:8081/.well-known/jwks.json" || c.UserRatePerMin != 300 {
		t.Fatalf("%+v", c)
	}
}

func TestCF02_DevSwitchesRejectedOutsideDevelopment(t *testing.T) {
	for key, val := range map[string]string{"DEV_HS256_SECRET": "x", "EXPOSE_DEV_ROUTES": "true", "CORS_ALLOWED_ORIGINS": "*"} {
		t.Run(key, func(t *testing.T) {
			base(t)
			t.Setenv(key, val)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ENV=development") {
				t.Fatalf("expected rejection, got %v", err)
			}
			t.Setenv("ENV", "development")
			if _, err := Load(); err != nil {
				t.Fatalf("development: %v", err)
			}
		})
	}
}

func TestCF03_MissingSecretsFail(t *testing.T) {
	base(t)
	t.Setenv("NOTIFICACIONES_SECRET", "")
	t.Setenv("INTERNAL_SECRET", "")
	t.Setenv("AUTH_TENANT_HMAC_SECRET", "short")
	_, err := Load()
	for _, want := range []string{"NOTIFICACIONES_SECRET", "INTERNAL_SECRET", "AUTH_TENANT_HMAC_SECRET"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %s in %v", want, err)
		}
	}
}
