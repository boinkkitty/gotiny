package config

import (
	"testing"
)

func TestEnvOr_Fallback(t *testing.T) {
	got := EnvOr("GOTINY_TEST_MISSING_VAR_XYZ", "default")
	if got != "default" {
		t.Errorf("expected default, got %q", got)
	}
}

func TestEnvOr_Set(t *testing.T) {
	t.Setenv("GOTINY_TEST_ENV_OR", "from_env")
	got := EnvOr("GOTINY_TEST_ENV_OR", "fallback")
	if got != "from_env" {
		t.Errorf("expected from_env, got %q", got)
	}
}
