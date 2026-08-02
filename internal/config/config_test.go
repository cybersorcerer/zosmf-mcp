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

func TestLoad_ServerWithoutUser_Tolerated(t *testing.T) {
	t.Setenv("ZOSMF_PASSWORD", "")
	yaml := `
systems:
  - name: LEGACY
    host: https://zosmf.example.com
`
	cfg, err := Load(writeConfig(t, yaml))
	if err != nil {
		t.Fatalf("Load must tolerate entries without user: %v", err)
	}
	if cfg.Systems[0].Auth.Type != "" {
		t.Errorf("Auth.Type = %q, want empty", cfg.Systems[0].Auth.Type)
	}
}

func TestLoad_CertFilesWithoutType_Fails(t *testing.T) {
	t.Setenv("ZOSMF_PASSWORD", "")
	yaml := `
systems:
  - name: SYSA
    host: https://zosmf.example.com
    auth:
      cert_file: /path/cert.pem
      key_file: /path/key.pem
`
	_, err := Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for cert files configured without auth type")
	}
	want := `system SYSA: certificate files configured but auth type missing — set "type: certificate"`
	if err.Error() != "validating config: "+want {
		t.Errorf("error = %q, want suffix %q", err.Error(), want)
	}
}

func TestResolveDefaultPath_CwdBeatsLegacy(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeFile(t, filepath.Join(cwd, ".smpe-zosmf.yaml"))
	writeFile(t, filepath.Join(cwd, ".zosmf-mcp-config.yaml"))

	path, deprecated, err := ResolveDefaultPath(cwd, home)
	if err != nil {
		t.Fatal(err)
	}
	if deprecated {
		t.Error("deprecated must be false when the new-name file is found first")
	}
	if path != filepath.Join(cwd, ".smpe-zosmf.yaml") {
		t.Errorf("path = %q", path)
	}
}

func TestResolveDefaultPath_EmptyHome_SkipsHomeCandidate(t *testing.T) {
	cwd := t.TempDir()
	want := filepath.Join(cwd, ".zosmf-mcp-config.yaml")
	writeFile(t, want)

	path, deprecated, err := ResolveDefaultPath(cwd, "")
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

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("servers: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func loadTestConfig(t *testing.T, yaml string) *Config {
	t.Helper()
	cfg, err := Load(writeConfig(t, yaml))
	if err != nil {
		t.Fatalf("loadTestConfig: %v", err)
	}
	return cfg
}

func loadTestConfigErr(t *testing.T, yaml string) (*Config, error) {
	t.Helper()
	return Load(writeConfig(t, yaml))
}

func TestProxyInheritedFromGlobal(t *testing.T) {
	cfg := loadTestConfig(t, `
proxy:
  enabled: true
  url: socks5://tunnel:1080
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
	if !cfg.Systems[0].Proxy.Active() {
		t.Fatal("expected proxy active on SYS1")
	}
	if got := cfg.Systems[0].Proxy.URL; got != "socks5://tunnel:1080" {
		t.Errorf("URL = %q, want socks5://tunnel:1080", got)
	}
}

func TestProxyPerSystemOverridesGlobal(t *testing.T) {
	cfg := loadTestConfig(t, `
proxy:
  enabled: true
  url: socks5://tunnel:1080
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
  - name: SYS2
    host: https://intern.lab
    user: IBMUSER
    proxy:
      enabled: false
`)
	if !cfg.Systems[0].Proxy.Active() {
		t.Error("SYS1: expected proxy active")
	}
	if cfg.Systems[1].Proxy.Active() {
		t.Error("SYS2: expected proxy inactive")
	}
}

func TestProxyAbsentMeansInactive(t *testing.T) {
	cfg := loadTestConfig(t, `
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
	if cfg.Systems[0].Proxy == nil {
		t.Fatal("Proxy must be non-nil after normalize")
	}
	if cfg.Systems[0].Proxy.Active() {
		t.Error("expected proxy inactive without proxy block")
	}
}

func TestProxyURLFromEnv(t *testing.T) {
	t.Setenv("ZOSMF_PROXY", "socks5://tunnel:1080")
	cfg := loadTestConfig(t, `
proxy:
  enabled: true
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
	if got := cfg.Systems[0].Proxy.URL; got != "socks5://tunnel:1080" {
		t.Errorf("URL = %q, want value from ZOSMF_PROXY", got)
	}
}

func TestProxyRejectsUnknownScheme(t *testing.T) {
	_, err := loadTestConfigErr(t, `
proxy:
  enabled: true
  url: ftp://tunnel:1080
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
	if err == nil {
		t.Fatal("expected error for scheme ftp")
	}
	if !strings.Contains(err.Error(), "ftp") {
		t.Errorf("error should name the bad scheme, got: %v", err)
	}
}

func TestSSHProxyJumpInherited(t *testing.T) {
	cfg := loadTestConfig(t, `
ssh_proxy_jump:
  enabled: true
  host: jump@jumphost.example.com
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
ssh_systems:
  - name: SYS1
    host: zos1.example.com
    username: ibmuser
`)
	if !cfg.SSHSystems[0].ProxyJump.Active() {
		t.Fatal("expected proxy jump active")
	}
	if got := cfg.SSHSystems[0].ProxyJump.Host; got != "jump@jumphost.example.com" {
		t.Errorf("Host = %q, want jump@jumphost.example.com", got)
	}
}
