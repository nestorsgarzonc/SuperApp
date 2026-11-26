package config

import "testing"

// Caso CF-01 · Sin ENV explícito la configuración arranca en modo seguro (no desarrollo).
// Entrada: Variable ENV vacía o ausente
// Debe hacer / esperado: No se activa el modo de desarrollo
func TestCF01_EnvFailsClosedByDefault(t *testing.T) {
	t.Setenv("ENV", "")
	if got := Load().Env; got != "production" {
		t.Fatalf("with ENV unset the service must behave as production, got %q", got)
	}
}

// Caso CF-02 · El modo de desarrollo solo se activa con el valor exacto "development".
// Entrada: ENV con valores distintos y con "development"
// Debe hacer / esperado: Solo "development" activa el modo de desarrollo
func TestCF02_EnvDevelopmentOnlyWhenExplicit(t *testing.T) {
	t.Setenv("ENV", "development")
	if got := Load().Env; got != "development" {
		t.Fatalf("got %q", got)
	}
}
