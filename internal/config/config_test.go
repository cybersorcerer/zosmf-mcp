package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestResolveDefaultPath_CwdFirst(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeFile(t, filepath.Join(cwd, ".smpe-zosmf.yaml"))
	writeFile(t, filepath.Join(home, ".config", "smpe_ls", ".smpe-zosmf.yaml"))

	path, deprecated, err := ResolveDefaultPath(cwd, home)
	if err != nil {
		t.Fatal(err)
	}
	if deprecated {
		t.Error("cwd .smpe-zosmf.yaml must not be deprecated")
	}
	if path != filepath.Join(cwd, ".smpe-zosmf.yaml") {
		t.Errorf("path = %q", path)
	}
}

func TestResolveDefaultPath_HomeSecond(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	want := filepath.Join(home, ".config", "smpe_ls", ".smpe-zosmf.yaml")
	writeFile(t, want)

	path, deprecated, err := ResolveDefaultPath(cwd, home)
	if err != nil || deprecated || path != want {
		t.Errorf("path=%q deprecated=%v err=%v", path, deprecated, err)
	}
}

func TestResolveDefaultPath_LegacyThird(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	want := filepath.Join(cwd, ".zosmf-mcp-config.yaml")
	writeFile(t, want)

	path, deprecated, err := ResolveDefaultPath(cwd, home)
	if err != nil {
		t.Fatal(err)
	}
	if !deprecated {
		t.Error("legacy file must report deprecated=true")
	}
	if path != want {
		t.Errorf("path = %q", path)
	}
}

func TestResolveDefaultPath_NothingFound(t *testing.T) {
	_, _, err := ResolveDefaultPath(t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("expected error when no config exists")
	}
	for _, frag := range []string{".smpe-zosmf.yaml", "smpe_ls", ".zosmf-mcp-config.yaml"} {
		if !strings.Contains(err.Error(), frag) {
			t.Errorf("error %q does not mention %q", err.Error(), frag)
		}
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("servers: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
