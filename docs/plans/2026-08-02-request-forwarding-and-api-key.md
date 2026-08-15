# Request-Forwarding und API-Key Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** z/OSMF-REST-Calls und ZOAU-SSH-Verbindungen konfigurierbar über einen Jump-Host leiten und die HTTP-Transports mit einem API-Key absichern, damit der Server in Docker ohne direkten z/OS-Netzzugang laufen kann.

**Architecture:** Alle REST-Calls laufen durch genau einen `http.Client` (`internal/zosmf/client.go:146`), daher genügt dort ein `Transport.Proxy`. ZOAU nutzt `exec.Command("ssh")`, daher genügt dort ein `-J`-Argument. Beide HTTP-Transports teilen sich `NewHTTPTransport`, daher genügt ein Middleware-Wrap. Kein Tool-Handler wird angefasst.

**Tech Stack:** Go 1.26.1, Standardbibliothek (`net/http`, `crypto/subtle`, `os/exec`), `gopkg.in/yaml.v3`, Docker Compose.

**Spec:** `docs/2026-08-02-request-forwarding-and-api-key-design.md`

## Global Constraints

- Keine neuen Go-Dependencies. `go.mod` bleibt bei `github.com/google/uuid v1.6.0` und `gopkg.in/yaml.v3 v3.0.1`.
- Rückwärtskompatibilität: fehlender `proxy`-Block, fehlender `ssh_proxy_jump`-Block und leerer `api_key` ergeben exakt das heutige Verhalten. Bestehende Tests müssen unverändert grün bleiben.
- Alle Builds und Tests über `make` (`make build`, `make test`, `make vet`, `make fmt`).
- Tests laufen mit Race-Detector: `make test` bzw. `go test -race -count=1 ./internal/...`.
- Commit-Messages **niemals** mit `Co-Authored-By`-Trailer.
- Erlaubte Proxy-Schemas: `http`, `https`, `socks5`.
- Bestehender Codestil: keine Umstrukturierung angrenzenden Codes, keine Kommentare zu Selbstverständlichem.

## File Structure

| Datei | Verantwortung | Status |
|---|---|---|
| `internal/config/config.go` | `ProxyConfig`, `SSHProxyJumpConfig`, `APIKey`/`AllowInsecure` in `HTTPConfig`, Vererbung in `normalize()`, Validierung in `validate()` | Modify |
| `internal/config/config_test.go` | Tests für Vererbung, Env-Var, Schema-Validierung | Modify |
| `internal/zosmf/client.go` | `Transport.Proxy` in `NewClient` setzen | Modify |
| `internal/zosmf/client_proxy_test.go` | Tests für gesetzten/nicht gesetzten Proxy | Create |
| `internal/zosmf/ssh.go` | `sshBaseArgs` Helper mit `-J`, genutzt von `RunSSH` und `RunSSHDetailed` | Modify |
| `internal/zosmf/ssh_test.go` | Tests für Argumentliste | Create |
| `internal/mcp/transport_http.go` | `apiKey`-Feld, `requireAPIKey`-Middleware | Modify |
| `internal/mcp/transport_http_auth_test.go` | Tests für 401/Durchreichen | Create |
| `cmd/zosmf-mcp/main.go` | Call-Sites, Startabbruch ohne Key | Modify |
| `Dockerfile` | Multi-Stage-Build mit `openssh-client` | Create |
| `docker-compose.yml` | `tunnel`-Sidecar + `zosmf-mcp` | Create |
| `.dockerignore` | Build-Kontext klein halten | Create |
| `.env.example` | Vorlage für Secrets | Create |
| `.gitignore` | `.smpe-zosmf.yaml` ergänzen | Modify |
| `Makefile` | `apikey`, `docker-build`, `docker-up`, `docker-logs` | Modify |
| `README.md` | Abschnitte Forwarding, API-Key, Docker | Modify |

Reihenfolge: Config zuerst (alle anderen Tasks konsumieren die Typen), dann die drei unabhängigen Verbraucher (Client, SSH, Transport), dann Verdrahtung in `main.go`, dann Docker, dann Doku.

---

### Task 1: Config-Typen für Proxy und ProxyJump

**Files:**
- Modify: `internal/config/config.go:13-60` (Structs), `:109-168` (`normalize`), `:170-230` (`validate`)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nichts.
- Produces:
  - `type ProxyConfig struct { Enabled bool; URL string }` mit yaml-Tags `enabled`, `url`
  - `type SSHProxyJumpConfig struct { Enabled bool; Host string }` mit yaml-Tags `enabled`, `host`
  - `Config.Proxy ProxyConfig` (yaml `proxy`), `Config.SSHProxyJump SSHProxyJumpConfig` (yaml `ssh_proxy_jump`)
  - `SystemConfig.Proxy *ProxyConfig` (yaml `proxy`, Pointer für "nicht gesetzt")
  - `SSHSystemConfig.ProxyJump *SSHProxyJumpConfig` (yaml `proxy_jump`)
  - `(ProxyConfig).Active() bool` — true wenn `Enabled && URL != ""`
  - `(SSHProxyJumpConfig).Active() bool` — true wenn `Enabled && Host != ""`
  - Nach `normalize()` ist `SystemConfig.Proxy` immer non-nil und aufgelöst; ebenso `SSHSystemConfig.ProxyJump`.

- [ ] **Step 1: Write the failing tests**

In `internal/config/config_test.go` ergänzen. Bestehendes Test-Muster in der Datei prüfen und übernehmen (Hilfsfunktion zum Schreiben einer temporären YAML-Datei ist dort vorhanden — wiederverwenden statt neu bauen).

```go
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
    host: https://zosmf-internal.example.com
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
```

Falls `loadTestConfig`/`loadTestConfigErr` in der Datei noch nicht existieren, diese Helfer ergänzen — `loadTestConfig` schreibt den String in `t.TempDir()`, ruft `Load` auf und ruft `t.Fatal` bei Fehler; `loadTestConfigErr` gibt `(*Config, error)` zurück, ohne zu failen.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -race -count=1 ./internal/config/ -run 'Proxy'`
Expected: Compile-Fehler, `cfg.Systems[0].Proxy undefined`.

- [ ] **Step 3: Add the types**

In `internal/config/config.go`:

```go
type ProxyConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
}

// Active reports whether requests should be routed through the proxy.
func (p ProxyConfig) Active() bool {
	return p.Enabled && p.URL != ""
}

type SSHProxyJumpConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
}

// Active reports whether SSH connections should be routed via a jump host.
func (s SSHProxyJumpConfig) Active() bool {
	return s.Enabled && s.Host != ""
}
```

`Config` (`:13-18`) ergänzen um:

```go
	Proxy        ProxyConfig        `yaml:"proxy"`
	SSHProxyJump SSHProxyJumpConfig `yaml:"ssh_proxy_jump"`
```

`SystemConfig` (`:20-41`) ergänzen um `Proxy *ProxyConfig \`yaml:"proxy"\``.

`SSHSystemConfig` (`:62-69`) ergänzen um `ProxyJump *SSHProxyJumpConfig \`yaml:"proxy_jump"\``.

- [ ] **Step 4: Implement inheritance in normalize()**

In `normalize()` (`:109`), am Anfang die Env-Var auflösen:

```go
	if c.Proxy.URL == "" {
		c.Proxy.URL = os.Getenv("ZOSMF_PROXY")
	}
```

In der `for i := range c.Systems`-Schleife (`:112`) ans Ende:

```go
		// proxy: global default, per-system override wins
		if s.Proxy == nil {
			global := c.Proxy
			s.Proxy = &global
		}
```

In der `for i := range c.SSHSystems`-Schleife (`:163`):

```go
		if c.SSHSystems[i].ProxyJump == nil {
			global := c.SSHProxyJump
			c.SSHSystems[i].ProxyJump = &global
		}
```

Der Kopiervorgang über eine lokale Variable ist nötig, damit die Systeme nicht denselben Pointer teilen.

- [ ] **Step 5: Implement validation**

`net/url` zu den Imports hinzufügen. In `validate()` (`:170`) vor der Systemschleife:

```go
	if err := validateProxy(c.Proxy); err != nil {
		return err
	}
```

In der Systemschleife nach der Auth-Prüfung:

```go
		if sys.Proxy != nil {
			if err := validateProxy(*sys.Proxy); err != nil {
				return fmt.Errorf("system %s: %w", sys.Name, err)
			}
		}
```

Und die Hilfsfunktion:

```go
func validateProxy(p ProxyConfig) error {
	if p.URL == "" {
		return nil
	}
	u, err := url.Parse(p.URL)
	if err != nil {
		return fmt.Errorf("parsing proxy URL: %w", err)
	}
	switch u.Scheme {
	case "http", "https", "socks5":
	default:
		return fmt.Errorf("proxy URL scheme must be http, https or socks5, got '%s'", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("proxy URL is missing a host: %s", p.URL)
	}
	return nil
}
```

`validate()` läuft nach `normalize()`, daher wird die per-System-Prüfung auch für geerbte Werte ausgeführt — doppelt, aber harmlos und billig.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test -race -count=1 ./internal/config/`
Expected: PASS, auch alle vorher bestehenden Tests.

- [ ] **Step 7: Format, vet, commit**

```bash
make fmt && make vet
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): proxy and ssh_proxy_jump settings with per-system override"
```

---

### Task 2: Proxy im z/OSMF-HTTP-Client

**Files:**
- Modify: `internal/zosmf/client.go:59-110` (`NewClient`)
- Test: `internal/zosmf/client_proxy_test.go` (Create)

**Interfaces:**
- Consumes: `config.SystemConfig.Proxy *config.ProxyConfig`, `(config.ProxyConfig).Active()` aus Task 1.
- Produces: keine neue exportierte API. `NewClient` gibt bei ungültiger Proxy-URL einen Fehler zurück.

- [ ] **Step 1: Write the failing test**

Neue Datei `internal/zosmf/client_proxy_test.go`:

```go
package zosmf

import (
	"net/http"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
)

func proxyForClient(t *testing.T, c *Client) *http.Request {
	t.Helper()
	tr, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T, want *http.Transport", c.httpClient.Transport)
	}
	if tr.Proxy == nil {
		return nil
	}
	req, err := http.NewRequest(http.MethodGet, "https://zos1.example.com/zosmf/info", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy() returned error: %v", err)
	}
	if u == nil {
		return nil
	}
	return &http.Request{URL: u}
}

func TestNewClientSetsProxyWhenActive(t *testing.T) {
	c, err := NewClient(config.SystemConfig{
		Name: "SYS1",
		Host: "https://zos1.example.com",
		Auth: config.AuthConfig{Type: "basic", Username: "IBMUSER", Password: "secret"},
		Proxy: &config.ProxyConfig{Enabled: true, URL: "socks5://tunnel:1080"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	got := proxyForClient(t, c)
	if got == nil {
		t.Fatal("expected proxy to be configured")
	}
	if got.URL.String() != "socks5://tunnel:1080" {
		t.Errorf("proxy = %q, want socks5://tunnel:1080", got.URL.String())
	}
}

func TestNewClientNoProxyWhenDisabled(t *testing.T) {
	c, err := NewClient(config.SystemConfig{
		Name: "SYS1",
		Host: "https://zos1.example.com",
		Auth: config.AuthConfig{Type: "basic", Username: "IBMUSER", Password: "secret"},
		Proxy: &config.ProxyConfig{Enabled: false, URL: "socks5://tunnel:1080"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := proxyForClient(t, c); got != nil {
		t.Errorf("expected no proxy, got %v", got.URL)
	}
}

func TestNewClientNoProxyWhenAbsent(t *testing.T) {
	c, err := NewClient(config.SystemConfig{
		Name: "SYS1",
		Host: "https://zos1.example.com",
		Auth: config.AuthConfig{Type: "basic", Username: "IBMUSER", Password: "secret"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := proxyForClient(t, c); got != nil {
		t.Errorf("expected no proxy, got %v", got.URL)
	}
}

func TestNewClientRejectsBadProxyURL(t *testing.T) {
	_, err := NewClient(config.SystemConfig{
		Name: "SYS1",
		Host: "https://zos1.example.com",
		Auth: config.AuthConfig{Type: "basic", Username: "IBMUSER", Password: "secret"},
		Proxy: &config.ProxyConfig{Enabled: true, URL: "://nonsense"},
	})
	if err == nil {
		t.Fatal("expected error for malformed proxy URL")
	}
}
```

Hinweis: Der Test liegt im Package `zosmf` (nicht `zosmf_test`), um an das unexportierte Feld `httpClient` zu kommen. Das entspricht dem Muster in `client_auth_test.go`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -race -count=1 ./internal/zosmf/ -run 'Proxy'`
Expected: Compile-Fehler, `unknown field Proxy in struct literal` — bis Task 1 gemerged ist. Ist Task 1 vorhanden, schlägt `TestNewClientSetsProxyWhenActive` mit "expected proxy to be configured" fehl.

- [ ] **Step 3: Implement**

In `internal/zosmf/client.go` `net/url` importieren. Den Block bei `:92-97` ersetzen:

```go
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	if sys.Proxy != nil && sys.Proxy.Active() {
		proxyURL, err := url.Parse(sys.Proxy.URL)
		if err != nil {
			return nil, fmt.Errorf("parsing proxy URL for system %s: %w", sys.Name, err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	httpClient := &http.Client{
		Jar:       jar,
		Transport: transport,
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race -count=1 ./internal/zosmf/`
Expected: PASS, inklusive der bestehenden Tests in `client_auth_test.go` und `datasets_test.go`.

- [ ] **Step 5: Format, vet, commit**

```bash
make fmt && make vet
git add internal/zosmf/client.go internal/zosmf/client_proxy_test.go
git commit -m "feat(zosmf): route REST calls through configured proxy"
```

---

### Task 3: ProxyJump für ZOAU-SSH

**Files:**
- Modify: `internal/zosmf/ssh.go:33-73` (`RunSSH`), `:82-...` (`RunSSHDetailed`)
- Test: `internal/zosmf/ssh_test.go` (Create)

**Interfaces:**
- Consumes: `config.SSHSystemConfig.ProxyJump *config.SSHProxyJumpConfig` aus Task 1.
- Produces: `func sshBaseArgs(cfg config.SSHSystemConfig) []string` — unexportiert, liefert die gemeinsamen ssh-Argumente inklusive `-J`, ohne Ziel und Kommando.

`RunSSH` und `RunSSHDetailed` bauen die Argumentliste heute identisch (`ssh.go:37-44` und `:90-...`). Statt `-J` an beiden Stellen zu duplizieren, wird der gemeinsame Teil in `sshBaseArgs` gezogen. Das ist die einzige Umstrukturierung in diesem Plan und dient direkt der Anforderung.

- [ ] **Step 1: Write the failing test**

Neue Datei `internal/zosmf/ssh_test.go`:

```go
package zosmf

import (
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
)

func hasFlagValue(args []string, flag, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

func TestSSHBaseArgsIncludesJumpWhenActive(t *testing.T) {
	args := sshBaseArgs(config.SSHSystemConfig{
		Name:      "SYS1",
		Host:      "zos1.example.com",
		Port:      22,
		Username:  "ibmuser",
		ProxyJump: &config.SSHProxyJumpConfig{Enabled: true, Host: "jump@jumphost.example.com"},
	})
	if !hasFlagValue(args, "-J", "jump@jumphost.example.com") {
		t.Errorf("expected -J jump@jumphost.example.com in %v", args)
	}
}

func TestSSHBaseArgsOmitsJumpWhenInactive(t *testing.T) {
	for _, tc := range []struct {
		name string
		jump *config.SSHProxyJumpConfig
	}{
		{"absent", nil},
		{"disabled", &config.SSHProxyJumpConfig{Enabled: false, Host: "jump@jumphost.example.com"}},
		{"no host", &config.SSHProxyJumpConfig{Enabled: true, Host: ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := sshBaseArgs(config.SSHSystemConfig{
				Name: "SYS1", Host: "zos1.example.com", Port: 22,
				Username: "ibmuser", ProxyJump: tc.jump,
			})
			if strings.Contains(strings.Join(args, " "), "-J") {
				t.Errorf("expected no -J in %v", args)
			}
		})
	}
}

func TestSSHBaseArgsKeepsExistingOptions(t *testing.T) {
	args := sshBaseArgs(config.SSHSystemConfig{
		Name: "SYS1", Host: "zos1.example.com", Port: 2222,
		Username: "ibmuser", KeyFile: "/keys/zos1",
	})
	if !hasFlagValue(args, "-p", "2222") {
		t.Errorf("expected -p 2222 in %v", args)
	}
	if !hasFlagValue(args, "-i", "/keys/zos1") {
		t.Errorf("expected -i /keys/zos1 in %v", args)
	}
	if !hasFlagValue(args, "-o", "BatchMode=yes") {
		t.Errorf("expected -o BatchMode=yes in %v", args)
	}
	if !hasFlagValue(args, "-o", "StrictHostKeyChecking=accept-new") {
		t.Errorf("expected -o StrictHostKeyChecking=accept-new in %v", args)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -race -count=1 ./internal/zosmf/ -run 'SSHBaseArgs'`
Expected: Compile-Fehler, `undefined: sshBaseArgs`.

- [ ] **Step 3: Implement the helper**

In `internal/zosmf/ssh.go` vor `RunSSH` einfügen:

```go
// sshBaseArgs builds the ssh options shared by RunSSH and RunSSHDetailed.
// The target and the remote command are appended by the caller.
func sshBaseArgs(cfg config.SSHSystemConfig) []string {
	args := []string{
		"-p", fmt.Sprintf("%d", cfg.Port),
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if cfg.KeyFile != "" {
		args = append(args, "-i", cfg.KeyFile)
	}
	if cfg.ProxyJump != nil && cfg.ProxyJump.Active() {
		args = append(args, "-J", cfg.ProxyJump.Host)
	}
	return args
}
```

- [ ] **Step 4: Use the helper in both functions**

In `RunSSH` die Zeilen `:37-44` ersetzen durch:

```go
	args := sshBaseArgs(cfg)
```

In `RunSSHDetailed` den gleichlautenden Block ebenso ersetzen. Die restlichen Zeilen (Environment-Exports, Anhängen von Ziel und Kommando) bleiben unverändert.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -race -count=1 ./internal/zosmf/`
Expected: PASS.

- [ ] **Step 6: Format, vet, commit**

```bash
make fmt && make vet
git add internal/zosmf/ssh.go internal/zosmf/ssh_test.go
git commit -m "feat(zoau): optional ssh proxy jump for remote command execution"
```

---

### Task 4: API-Key-Middleware im HTTP-Transport

**Files:**
- Modify: `internal/mcp/transport_http.go:17-24` (Struct), `:42-48` (`NewHTTPTransport`), `:50-59` (`Run`)
- Test: `internal/mcp/transport_http_auth_test.go` (Create)

**Interfaces:**
- Consumes: nichts aus früheren Tasks.
- Produces:
  - `func NewHTTPTransport(addr string, logger *log.Logger, apiKey string) *HTTPTransport` — **geänderte Signatur**, dritter Parameter neu. Task 5 passt die Call-Sites an.
  - `func requireAPIKey(key string, next http.Handler) http.Handler` — unexportiert.

- [ ] **Step 1: Write the failing test**

Neue Datei `internal/mcp/transport_http_auth_test.go`:

```go
package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("reached"))
	})
}

func TestRequireAPIKeyRejectsMissingHeader(t *testing.T) {
	h := requireAPIKey("s3cret", okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got == "" {
		t.Error("expected WWW-Authenticate header")
	}
	if rec.Body.String() == "reached" {
		t.Error("handler must not be reached")
	}
}

func TestRequireAPIKeyRejectsWrongKey(t *testing.T) {
	h := requireAPIKey("s3cret", okHandler())
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestRequireAPIKeyAcceptsCorrectKey(t *testing.T) {
	h := requireAPIKey("s3cret", okHandler())
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer s3cret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "reached" {
		t.Error("handler should have been reached")
	}
}

func TestRequireAPIKeyDisabledWhenEmpty(t *testing.T) {
	h := requireAPIKey("", okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 when no key configured", rec.Code)
	}
}

func TestRequireAPIKeyRejectsBareKeyWithoutBearer(t *testing.T) {
	h := requireAPIKey("s3cret", okHandler())
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "s3cret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 — only the Bearer scheme is accepted", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -race -count=1 ./internal/mcp/ -run 'RequireAPIKey'`
Expected: Compile-Fehler, `undefined: requireAPIKey`.

- [ ] **Step 3: Implement the middleware**

In `internal/mcp/transport_http.go` die Imports um `crypto/subtle` und `strings` ergänzen, dann:

```go
// requireAPIKey rejects requests without a matching bearer token. An empty key
// disables the check, which keeps stdio-only setups unaffected.
func requireAPIKey(key string, next http.Handler) http.Handler {
	if key == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="zosmf-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

`subtle.ConstantTimeCompare` statt `==`, damit die Antwortzeit den Key nicht zeichenweise verrät. `strings.CutPrefix` stellt sicher, dass ein nackter Key ohne `Bearer `-Präfix abgelehnt wird.

- [ ] **Step 4: Wire it into the transport**

`HTTPTransport` (`:17-24`) um ein Feld ergänzen:

```go
	apiKey  string
```

`NewHTTPTransport` (`:42`) anpassen:

```go
func NewHTTPTransport(addr string, logger *log.Logger, apiKey string) *HTTPTransport {
	return &HTTPTransport{
		addr:     addr,
		logger:   logger,
		apiKey:   apiKey,
		sessions: make(map[string]*session),
	}
}
```

In `Run` (`:56-59`):

```go
	server := &http.Server{
		Addr:    t.addr,
		Handler: requireAPIKey(t.apiKey, mux),
	}
```

Die beiden Call-Sites in `cmd/zosmf-mcp/main.go:163` und `:183` brechen jetzt den Build — das ist Task 5. Zum Kompilieren der Tests in diesem Task reicht `go test ./internal/mcp/`, da `main.go` in einem anderen Package liegt.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -race -count=1 ./internal/mcp/`
Expected: PASS, auch die bestehenden Tests in `logging_test.go`.

- [ ] **Step 6: Commit**

```bash
gofmt -s -w internal/mcp/
git add internal/mcp/transport_http.go internal/mcp/transport_http_auth_test.go
git commit -m "feat(mcp): bearer token auth for the HTTP transports"
```

`make vet` wird hier noch fehlschlagen, weil `main.go` die alte Signatur nutzt. Das ist erwartet und wird in Task 5 behoben.

---

### Task 5: Verdrahtung in main.go und Startabbruch ohne Key

**Files:**
- Modify: `internal/config/config.go:57-60` (`HTTPConfig`), `cmd/zosmf-mcp/main.go:154-192`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: `NewHTTPTransport(addr, logger, apiKey)` aus Task 4.
- Produces: `HTTPConfig.APIKey string` (yaml `api_key`), `HTTPConfig.AllowInsecure bool` (yaml `allow_insecure`).

- [ ] **Step 1: Write the failing test**

In `internal/config/config_test.go`:

```go
func TestHTTPTransportRequiresAPIKey(t *testing.T) {
	_, err := loadTestConfigErr(t, `
transport:
  streamable_http:
    enabled: true
    listen: "0.0.0.0:8081"
servers:
  - name: SYS1
    host: https://zos1.example.com
    user: IBMUSER
`)
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
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -race -count=1 ./internal/config/ -run 'APIKey|Insecure|StdioOnly'`
Expected: `TestHTTPTransportRequiresAPIKey` schlägt fehl mit "expected error".

- [ ] **Step 3: Extend HTTPConfig and validate**

In `internal/config/config.go` `HTTPConfig` (`:57-60`):

```go
type HTTPConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Listen        string `yaml:"listen"`
	APIKey        string `yaml:"api_key"`
	AllowInsecure bool   `yaml:"allow_insecure"`
}
```

In `validate()` nach der Proxy-Prüfung:

```go
	if err := validateHTTPAuth("http", c.Transport.HTTP); err != nil {
		return err
	}
	if err := validateHTTPAuth("streamable_http", c.Transport.StreamableHTTP); err != nil {
		return err
	}
```

Und die Hilfsfunktion:

```go
func validateHTTPAuth(name string, h HTTPConfig) error {
	if !h.Enabled || h.AllowInsecure || h.APIKey != "" {
		return nil
	}
	return fmt.Errorf("transport %s: api_key is required — set it, or set allow_insecure: true to run without authentication", name)
}
```

- [ ] **Step 4: Run config tests**

Run: `go test -race -count=1 ./internal/config/`
Expected: PASS.

- [ ] **Step 5: Fix the call sites in main.go**

`cmd/zosmf-mcp/main.go:163`:

```go
		t := mcp.NewHTTPTransport(addr, logger, cfg.Transport.HTTP.APIKey)
```

`cmd/zosmf-mcp/main.go:183`:

```go
		t := mcp.NewHTTPTransport(addr, logger, cfg.Transport.StreamableHTTP.APIKey)
```

Direkt nach dem `switch *transport` in `main.go` (nach Zeile `:132`) den Flag-Fall abdecken, den die Config-Validierung nicht sieht: `--transport http` kann einen Transport aktivieren, der in der Config deaktiviert und daher ungeprüft ist.

```go
	if useHTTP && cfg.Transport.HTTP.APIKey == "" && !cfg.Transport.HTTP.AllowInsecure {
		fmt.Fprintf(os.Stderr, "http transport needs transport.http.api_key (or allow_insecure: true)\n")
		os.Exit(1)
	}
	if useStreamableHTTP && cfg.Transport.StreamableHTTP.APIKey == "" && !cfg.Transport.StreamableHTTP.AllowInsecure {
		fmt.Fprintf(os.Stderr, "streamable-http transport needs transport.streamable_http.api_key (or allow_insecure: true)\n")
		os.Exit(1)
	}
```

Bei aktivem `allow_insecure` zusätzlich warnen, damit der Zustand in den Logs sichtbar ist — jeweils im `if useHTTP {` bzw. `if useStreamableHTTP {` Block, direkt nach der `logger.Printf("starting ...")`-Zeile:

```go
		if cfg.Transport.HTTP.APIKey == "" {
			logger.Println("WARNING: HTTP transport running without authentication")
		}
```

analog für Streamable HTTP mit `cfg.Transport.StreamableHTTP.APIKey`.

- [ ] **Step 6: Full build and test**

Run: `make fmt && make vet && make test && make build`
Expected: alles grün, Binary in `bin/zosmf-mcp`.

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go cmd/zosmf-mcp/main.go
git commit -m "feat: require api_key when an HTTP transport is enabled"
```

---

### Task 6: Docker-Setup

**Files:**
- Create: `Dockerfile`, `docker-compose.yml`, `.dockerignore`, `.env.example`
- Modify: `.gitignore`, `Makefile`

**Interfaces:**
- Consumes: `ZOSMF_PROXY`, `ZOSMF_PASSWORD`, `MCP_API_KEY` aus der Env; Config unter `/config/.smpe-zosmf.yaml`.
- Produces: keine Go-API.

- [ ] **Step 1: Write the Dockerfile**

```dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=docker
ARG COMMIT=unknown
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/zosmf-mcp ./cmd/zosmf-mcp

FROM alpine:3.21
RUN apk add --no-cache ca-certificates openssh-client
COPY --from=build /out/zosmf-mcp /usr/local/bin/zosmf-mcp
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/zosmf-mcp", "--config", "/config/.smpe-zosmf.yaml"]
```

`openssh-client` wird für die ZOAU-Tools benötigt, die über `exec.Command("ssh", ...)` laufen.

- [ ] **Step 2: Write .dockerignore**

```text
bin/
dist/
.git/
.env
secrets/
docs/
*.md
.smpe-zosmf.yaml
.zosmf-mcp-config.yaml
```

- [ ] **Step 3: Write docker-compose.yml**

```yaml
services:
  tunnel:
    image: alpine/openssh
    command: >
      ssh -N -D 0.0.0.0:1080 -i /key
      -o ExitOnForwardFailure=yes
      -o ServerAliveInterval=30
      -o ServerAliveCountMax=3
      -o StrictHostKeyChecking=accept-new
      ${JUMP_HOST}
    volumes:
      - ./secrets/jump_key:/key:ro
    restart: unless-stopped

  zosmf-mcp:
    build:
      context: .
      args:
        VERSION: docker
    depends_on:
      - tunnel
    ports:
      - "8081:8081"
    volumes:
      - ./.smpe-zosmf.yaml:/config/.smpe-zosmf.yaml:ro
      - ./secrets:/keys:ro
    environment:
      ZOSMF_PASSWORD: ${ZOSMF_PASSWORD}
      ZOSMF_PROXY: socks5://tunnel:1080
      MCP_API_KEY: ${MCP_API_KEY}
    restart: unless-stopped
```

Port 1080 wird bewusst nicht gemappt — der SOCKS5-Endpunkt bleibt im Docker-internen Netz.

- [ ] **Step 4: Write .env.example**

```bash
# Kopieren nach .env und ausfüllen. .env ist in .gitignore.
JUMP_HOST=jump@jumphost.example.com
ZOSMF_PASSWORD=
MCP_API_KEY=
```

- [ ] **Step 5: Extend .gitignore**

Unter `# Config with secrets` neben `.zosmf-mcp-config.yaml` ergänzen:

```text
.smpe-zosmf.yaml
secrets/
```

- [ ] **Step 6: Extend the Makefile**

`.PHONY`-Zeile (`:16`) um `apikey docker-build docker-up docker-down docker-logs` erweitern, dann am Dateiende:

```makefile
apikey:
	@openssl rand -base64 32

docker-build:
	docker compose build

docker-up: docker-build
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f
```

- [ ] **Step 7: Verify the build**

```bash
make docker-build
```

Expected: Image baut durch. Falls `go.sum` fehlt, `go mod tidy` laufen lassen und die `COPY go.mod go.sum ./`-Zeile im Dockerfile prüfen.

Danach prüfen, dass `ssh` im Image liegt:

```bash
docker compose run --rm --entrypoint sh zosmf-mcp -c 'ssh -V'
```

Expected: eine OpenSSH-Versionsausgabe.

- [ ] **Step 8: Commit**

```bash
git add Dockerfile docker-compose.yml .dockerignore .env.example .gitignore Makefile
git commit -m "build: docker image and compose setup with ssh tunnel sidecar"
```

---

### Task 7: Dokumentation

**Files:**
- Modify: `README.md`, `.smpe-zosmf.example.yaml` (falls vorhanden; sonst die im Repo vorhandene Beispieldatei — mit `ls *.example.yaml .*.example.yaml` prüfen)

**Interfaces:**
- Consumes: das fertige Verhalten aus Tasks 1-6.
- Produces: nichts.

- [ ] **Step 1: Extend the example config**

In der Beispiel-Config die neuen Blöcke ergänzen, auskommentiert, damit die Datei ohne Änderung weiter funktioniert:

```yaml
# Alle z/OSMF-REST-Calls über einen Proxy leiten (z.B. SOCKS5-Tunnel zu einem
# Jump-Host). Ohne diesen Block gehen alle Requests direkt raus.
# proxy:
#   enabled: true
#   url: ${ZOSMF_PROXY}          # socks5://tunnel:1080

# ZOAU-SSH-Verbindungen über einen Jump-Host leiten (ssh -J).
# ssh_proxy_jump:
#   enabled: true
#   host: jump@jumphost.example.com

# Einzelne Systeme vom Proxy ausnehmen:
# servers:
#   - name: SYS2
#     host: https://zosmf-internal.example.com:443
#     proxy:
#       enabled: false
```

- [ ] **Step 2: Add a README section**

Nach dem bestehenden Konfigurationsabschnitt einfügen. Inhalt:

- Wann Forwarding nötig ist: Server läuft ohne direkten Netzzugang zu den z/OS-Systemen.
- Das Topologie-Diagramm aus dem Spec (`docs/2026-08-02-request-forwarding-and-api-key-design.md`).
- Das Config-Schema aus Task 1 mit `proxy`, `ssh_proxy_jump` und dem per-System-Override.
- Hinweis, dass die TLS-Verbindung end-to-end bestehen bleibt und der Jump-Host nur verschlüsselte Bytes sieht.
- API-Key: Erzeugung per `make apikey`, Ablage in `.env`, Header `Authorization: Bearer <key>` im Client, Beispiel-`.mcp.json` aus dem Spec.
- Warnung: Über unverschlüsseltes HTTP schützt der Key gegen Portscans im LAN, aber nicht gegen Mitlesen — Key und z/OS-Kommandos gehen im Klartext über das Netz.
- Docker-Betrieb: `cp .env.example .env`, ausfüllen, `make docker-up`, `make docker-logs`.
- Hinweis, dass `depends_on` nur auf den Container-Start wartet und die ersten Requests nach einem Neustart fehlschlagen können, bis der Tunnel steht.

- [ ] **Step 3: Verify**

Run: `make test && make build`
Expected: PASS. Anschließend README gegen den Spec lesen: jede dort beschriebene Config-Option muss im README auftauchen.

- [ ] **Step 4: Commit**

```bash
git add README.md .smpe-zosmf.example.yaml
git commit -m "docs: document request forwarding, api key auth and docker setup"
```

---

## Self-Review

**Spec-Abdeckung:**

| Spec-Abschnitt | Task |
|---|---|
| Konfigurationsschema `proxy` global + pro System | 1 |
| `ZOSMF_PROXY` Env-Var | 1 |
| Schema-Validierung http/https/socks5 | 1 |
| `ssh_proxy_jump` | 1, 3 |
| `client.go:92` Transport.Proxy | 2 |
| `ssh.go:36` `-J` | 3 |
| `transport_http.go` Middleware | 4 |
| `main.go` Call-Sites + Startabbruch | 5 |
| `allow_insecure` | 5 |
| Dockerfile, compose, .dockerignore | 6 |
| Makefile `apikey`, docker-Targets | 6 |
| `.gitignore` für `.smpe-zosmf.yaml` | 6 |
| Testliste aus dem Spec | 1, 2, 3, 4, 5 |
| README | 7 |

Keine Lücken.

**Typkonsistenz:** `ProxyConfig`/`SSHProxyJumpConfig` in Task 1 definiert, in Tasks 2 und 3 unter denselben Namen konsumiert. `Active()` einheitlich in 1, 2, 3. `NewHTTPTransport` mit drittem Parameter in Task 4 definiert, in Task 5 so aufgerufen. `sshBaseArgs` in Task 3 definiert und dort genutzt. `validateProxy` und `validateHTTPAuth` in Task 1 bzw. 5.

**Bewusst abweichend vom Spec:** Der Spec nennt `strings.TrimPrefix` in der Middleware; Task 4 nutzt `strings.CutPrefix`, weil `TrimPrefix` einen nackten Key ohne `Bearer `-Präfix durchgelassen hätte. Der Spec listet keine `sshBaseArgs`-Extraktion; sie ist in Task 3 nötig, weil `RunSSH` und `RunSSHDetailed` die Argumentliste sonst doppelt pflegen müssten.

**Reihenfolge-Abhängigkeiten:** Tasks 2, 3 und 5 setzen Task 1 voraus. Task 5 setzt Task 4 voraus. Nach Task 4 ist der Repo-Zustand vorübergehend nicht baubar (`main.go` nutzt die alte Signatur) — Task 5 stellt das wieder her. Tasks 2, 3 und 4 sind untereinander unabhängig.
