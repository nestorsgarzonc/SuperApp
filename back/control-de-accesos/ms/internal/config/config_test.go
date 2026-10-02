package config

import "testing"

func TestEnvFailsClosedByDefault(t *testing.T) {
	t.Setenv("ENV", "")
	if got := Load().Env; got != "production" {
		t.Fatalf("with ENV unset the service must behave as production, got %q", got)
	}
}

func TestEnvDevelopmentOnlyWhenExplicit(t *testing.T) {
	t.Setenv("ENV", "development")
	if got := Load().Env; got != "development" {
		t.Fatalf("got %q", got)
	}
}
