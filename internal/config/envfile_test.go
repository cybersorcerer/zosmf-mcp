package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const twoSystemsYAML = `
servers:
  - name: MAIN
    host: https://zos1.example.com:443
    user: U
    tso_account: ${ACCT}
`

// withEnvFile writes the config and a .env beside it.
func withEnvFile(t *testing.T, env string, mode os.FileMode) string {
	t.Helper()
	path := writeConfig(t, twoSystemsYAML)
	envPath := filepath.Join(filepath.Dir(path), ".env")
	if err := os.WriteFile(envPath, []byte(env), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(envPath, mode); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ZOSMF_MAIN_PASSWORD", "ZOSMF_PASSWORD", "ACCT", "MCP_API_KEY"} {
		t.Setenv(name, "")
		_ = os.Unsetenv(name)
	}
	return path
}

// The server reads the .env next to its configuration, so no client has to
// be handed a password.
func TestLoadReadsTheEnvFileBesideTheConfig(t *testing.T) {
	path := withEnvFile(t, "ZOSMF_MAIN_PASSWORD=frompw\nexport ACCT='A1'\n", 0o600)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Systems[0]
	if s.Auth.Password != "frompw" || s.PasswordSource != "env-file" {
		t.Errorf("password %q from %q, want frompw from env-file", s.Auth.Password, s.PasswordSource)
	}
	if s.TSOAccount != "A1" {
		t.Errorf("tso_account = %q, want the quoted .env value without quotes", s.TSOAccount)
	}
}

func TestLoadPrefersTheProcessEnvironment(t *testing.T) {
	path := withEnvFile(t, "ZOSMF_MAIN_PASSWORD=fromfile\nACCT=A1\n", 0o600)
	t.Setenv("ZOSMF_MAIN_PASSWORD", "fromenv")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s := cfg.Systems[0]; s.Auth.Password != "fromenv" || s.PasswordSource != "system-env" {
		t.Errorf("password %q from %q, want fromenv from system-env", s.Auth.Password, s.PasswordSource)
	}
}

// A .env others can read is not trusted with passwords: ignored, and said so.
func TestLoadIgnoresAnEnvFileOthersCanRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no Unix permission bits")
	}
	path := withEnvFile(t, "ZOSMF_MAIN_PASSWORD=frompw\nACCT=A1\n", 0o644)
	t.Setenv("ACCT", "A1")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Systems[0].PasswordSource != "none" {
		t.Errorf("password source %q, want none", cfg.Systems[0].PasswordSource)
	}
	if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], "chmod 600") {
		t.Errorf("warnings = %q, want one naming chmod 600", cfg.Warnings)
	}
}

func TestLoadWithoutAnEnvFile(t *testing.T) {
	path := writeConfig(t, twoSystemsYAML)
	t.Setenv("ACCT", "A1")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EnvFile != "" || len(cfg.Warnings) != 0 {
		t.Errorf("EnvFile %q, warnings %q; want neither", cfg.EnvFile, cfg.Warnings)
	}
}
