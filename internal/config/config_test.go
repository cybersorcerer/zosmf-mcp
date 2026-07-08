package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const yamlNoPassword = `
systems:
  - name: SYSA
    host: https://zosmf.example.com
    user: USER1
`

func TestLoad_BasicAuthWithoutPassword_Succeeds(t *testing.T) {
	t.Setenv("ZOSMF_PASSWORD", "")

	cfg, err := Load(writeConfig(t, yamlNoPassword))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	sys := cfg.Systems[0]
	if sys.Auth.Type != "basic" {
		t.Errorf("Auth.Type = %q, want basic", sys.Auth.Type)
	}
	if sys.Auth.Password != "" {
		t.Errorf("Auth.Password = %q, want empty", sys.Auth.Password)
	}
}

func TestLoad_EnvPasswordFallback_StillApplies(t *testing.T) {
	t.Setenv("ZOSMF_PASSWORD", "envpw")

	cfg, err := Load(writeConfig(t, yamlNoPassword))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Systems[0].Auth.Password != "envpw" {
		t.Errorf("Auth.Password = %q, want envpw", cfg.Systems[0].Auth.Password)
	}
}

func TestLoad_BasicAuthWithoutUsername_Fails(t *testing.T) {
	t.Setenv("ZOSMF_PASSWORD", "")
	yaml := `
systems:
  - name: SYSA
    host: https://zosmf.example.com
    auth:
      type: basic
`
	if _, err := Load(writeConfig(t, yaml)); err == nil {
		t.Fatal("expected error for basic auth without username")
	}
}
