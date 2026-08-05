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

// Setting ZOSMF_PROXY is an explicit act, so it must enable the proxy on its
// own. The docker-compose setup relies on this: it passes the tunnel URL in
// the environment and mounts a config that has no proxy block at all. Without
// this, every request quietly bypasses the tunnel and runs into the timeout.
func TestProxyFromEnvIsActiveWithoutProxyBlock(t *testing.T) {
	t.Setenv("ZOSMF_PROXY", "socks5://tunnel:1080")
	cfg := loadTestConfig(t, `
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
	if !cfg.Proxy.Active() {
		t.Errorf("global proxy must be active from env alone, got %+v", cfg.Proxy)
	}
	if !cfg.Systems[0].Proxy.Active() {
		t.Errorf("system proxy must be active from env alone, got %+v", cfg.Systems[0].Proxy)
	}
}

// An explicit per-system proxy block must still win over the environment.
func TestProxyEnvDoesNotOverrideExplicitSystemProxy(t *testing.T) {
	t.Setenv("ZOSMF_PROXY", "socks5://tunnel:1080")
	cfg := loadTestConfig(t, `
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
    proxy:
      enabled: false
`)
	if cfg.Systems[0].Proxy.Active() {
		t.Error("per-system 'enabled: false' must win over ZOSMF_PROXY")
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

func TestHTTPTransportRequiresAPIKey(t *testing.T) {
	cfg := loadTestConfig(t, `
transport:
  streamable_http:
    enabled: true
    listen: "0.0.0.0:8081"
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
	err := cfg.ValidateHTTPAuth(false, true)
	if err == nil {
		t.Fatal("expected error: HTTP transport enabled without api_key")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("error should mention api_key, got: %v", err)
	}
}

func TestHTTPTransportAllowInsecureSkipsCheck(t *testing.T) {
	cfg := loadTestConfig(t, `
transport:
  streamable_http:
    enabled: true
    listen: "0.0.0.0:8081"
    allow_insecure: true
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
	if cfg.Transport.StreamableHTTP.APIKey != "" {
		t.Error("expected empty api_key")
	}
	if err := cfg.ValidateHTTPAuth(false, true); err != nil {
		t.Errorf("expected no error with allow_insecure, got: %v", err)
	}
}

func TestStdioOnlyNeedsNoAPIKey(t *testing.T) {
	cfg := loadTestConfig(t, `
transport:
  stdio: true
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
	if !cfg.Transport.Stdio {
		t.Error("expected stdio enabled")
	}
	if err := cfg.ValidateHTTPAuth(false, false); err != nil {
		t.Errorf("expected no error for stdio-only, got: %v", err)
	}
}

func TestHTTPEnabledWithoutAPIKeyLoadsFineWhenOnlyStdioStarted(t *testing.T) {
	cfg := loadTestConfig(t, `
transport:
  stdio: true
  http:
    enabled: true
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
	if err := cfg.ValidateHTTPAuth(false, false); err != nil {
		t.Errorf("expected no error when http is enabled in config but only stdio is started, got: %v", err)
	}
}

func TestProxyEnabledWithoutURLFails(t *testing.T) {
	_, err := loadTestConfigErr(t, `
proxy:
  enabled: true
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
	if err == nil {
		t.Fatal("expected error: proxy enabled without url")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Errorf("error should mention url, got: %v", err)
	}
}

func TestSSHProxyJumpEnabledWithoutHostFails(t *testing.T) {
	_, err := loadTestConfigErr(t, `
ssh_proxy_jump:
  enabled: true
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
	if err == nil {
		t.Fatal("expected error: ssh_proxy_jump enabled without host")
	}
	if !strings.Contains(err.Error(), "host") {
		t.Errorf("error should mention host, got: %v", err)
	}
}

func TestSSHProxyJumpPerSystemEnabledWithoutHostFails(t *testing.T) {
	_, err := loadTestConfigErr(t, `
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
ssh_systems:
  - name: SYS1
    host: zos1.example.com
    username: ibmuser
    proxy_jump:
      enabled: true
`)
	if err == nil {
		t.Fatal("expected error: per-system ssh proxy_jump enabled without host")
	}
	if !strings.Contains(err.Error(), "host") {
		t.Errorf("error should mention host, got: %v", err)
	}
}

func TestProxyPerSystemInheritsGlobalURL(t *testing.T) {
	cfg := loadTestConfig(t, `
proxy:
  enabled: true
  url: socks5://tunnel:1080
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
    proxy:
      enabled: true
`)
	if !cfg.Systems[0].Proxy.Active() {
		t.Fatal("expected proxy active on SYS1")
	}
	if got := cfg.Systems[0].Proxy.URL; got != "socks5://tunnel:1080" {
		t.Errorf("URL = %q, want inherited socks5://tunnel:1080", got)
	}
}

func TestSSHProxyJumpPerSystemInheritsGlobalHost(t *testing.T) {
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
    proxy_jump:
      enabled: true
`)
	if !cfg.SSHSystems[0].ProxyJump.Active() {
		t.Fatal("expected proxy jump active")
	}
	if got := cfg.SSHSystems[0].ProxyJump.Host; got != "jump@jumphost.example.com" {
		t.Errorf("Host = %q, want inherited jump@jumphost.example.com", got)
	}
}

// An unresolved ${VAR} used to survive into the parsed config as its own
// literal text, which surfaced much later as an unexplained 401 from z/OSMF.
// Failing at load time names the variable instead.
func TestLoadFailsOnUnresolvedEnvVars(t *testing.T) {
	_, err := loadTestConfigErr(t, `
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
    password: ${ZOSMF_SYS1_PASSWORD}
`)
	if err == nil {
		t.Fatal("expected an error for an unresolved ${VAR}")
	}
	if !strings.Contains(err.Error(), "ZOSMF_SYS1_PASSWORD") {
		t.Errorf("error must name the unresolved variable, got: %v", err)
	}
}

func TestLoadReportsAllUnresolvedEnvVars(t *testing.T) {
	_, err := loadTestConfigErr(t, `
servers:
  - name: SYS1
    host: ${SYS1_HOST}
    user: IBMUSER
    password: ${SYS1_PASSWORD}
`)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"SYS1_HOST", "SYS1_PASSWORD"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must name %q, got: %v", want, err)
		}
	}
}

func TestLoadAcceptsResolvedEnvVars(t *testing.T) {
	t.Setenv("ZOSMF_SYS1_PASSWORD", "secret")
	cfg := loadTestConfig(t, `
servers:
  - name: SYS1
    host: https://zos1.example.com
    auth:
      type: basic
      username: IBMUSER
      password: ${ZOSMF_SYS1_PASSWORD}
`)
	if cfg.Systems[0].Auth.Password != "secret" {
		t.Errorf("password not expanded, got %q", cfg.Systems[0].Auth.Password)
	}
}

// The docker-compose setup passes the API key in the environment so the
// mounted production config never has to carry it.
func TestAPIKeyFromEnv(t *testing.T) {
	t.Setenv("MCP_API_KEY", "key-from-env")
	cfg := loadTestConfig(t, `
transport:
  streamable_http:
    enabled: true
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
	if got := cfg.Transport.StreamableHTTP.APIKey; got != "key-from-env" {
		t.Errorf("StreamableHTTP.APIKey = %q, want key-from-env", got)
	}
	if got := cfg.Transport.HTTP.APIKey; got != "key-from-env" {
		t.Errorf("HTTP.APIKey = %q, want key-from-env", got)
	}
	if err := cfg.ValidateHTTPAuth(false, true); err != nil {
		t.Errorf("ValidateHTTPAuth must pass with the env key: %v", err)
	}
}

func TestAPIKeyInConfigWinsOverEnv(t *testing.T) {
	t.Setenv("MCP_API_KEY", "key-from-env")
	cfg := loadTestConfig(t, `
transport:
  streamable_http:
    enabled: true
    api_key: key-from-config
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
	if got := cfg.Transport.StreamableHTTP.APIKey; got != "key-from-config" {
		t.Errorf("APIKey = %q, want key-from-config", got)
	}
}

// --transport streamable-http turns a transport on even when the config file
// leaves it disabled — the container setup relies on exactly that. The api_key
// requirement must follow the transport that actually runs, not the file's
// enabled flag, or the server comes up unauthenticated on a published port.
func TestValidateHTTPAuthCoversForcedTransport(t *testing.T) {
	cfg := loadTestConfig(t, `
transport:
  stdio: true
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
	if err := cfg.ValidateHTTPAuth(false, true); err == nil {
		t.Error("forcing streamable_http without an api_key must be refused")
	}
	if err := cfg.ValidateHTTPAuth(true, false); err == nil {
		t.Error("forcing http without an api_key must be refused")
	}
	// stdio only stays unaffected.
	if err := cfg.ValidateHTTPAuth(false, false); err != nil {
		t.Errorf("stdio-only must not require an api_key: %v", err)
	}
}

func TestValidateHTTPAuthAcceptsForcedTransportWithEnvKey(t *testing.T) {
	t.Setenv("MCP_API_KEY", "key-from-env")
	cfg := loadTestConfig(t, `
transport:
  stdio: true
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
	if err := cfg.ValidateHTTPAuth(false, true); err != nil {
		t.Errorf("MCP_API_KEY must satisfy the forced transport: %v", err)
	}
}
