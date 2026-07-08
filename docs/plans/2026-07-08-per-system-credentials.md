# Per-System-Credentials Implementation Plan (zosmf-mcp)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** zosmf-mcp erlaubt Passwörter pro System zur Laufzeit über ein neues MCP-Tool `zosmf_set_password`; fehlende bzw. falsche Credentials sind über stabile Fehlermarker maschinell erkennbar.

**Architecture:** `zosmf.Client` bekommt einen Mutex-geschützten Passwort-Setter und wirft `NO_CREDENTIALS(<SYSTEM>):` vor dem HTTP-Request, wenn kein Passwort gesetzt ist; 401-Antworten werden als `AUTH_FAILED(<SYSTEM>):` markiert. Ein neues Tool `zosmf_set_password` (internal/tools/auth.go) setzt das Passwort in-memory. Die Config akzeptiert basic-auth-Systeme ohne Passwort.

**Tech Stack:** Go (siehe go.mod), Tests mit `net/http/httptest`, Build/Test via `make`.

**Spec:** `docs/2026-07-08-per-system-credentials-design.md`

## Global Constraints

- Branch: `feat/per-system-credentials` (existiert bereits, dort arbeiten).
- Fehlermarker exakt: `NO_CREDENTIALS(<SYSTEM>):` und `AUTH_FAILED(<SYSTEM>):` — `<SYSTEM>` uppercase. Marker sind **enthalten** in der Fehlermeldung (Fehler werden gewrappt), Tests prüfen mit `strings.Contains`. Format ist Schnittstellen-Vertrag mit der vscpdf-Extension — nicht ändern.
- Passwörter niemals loggen und niemals in Tool-Ergebnistexten ausgeben.
- Alle Builds/Tests mit make: `make test` (= `go test ./...`), `make build`.
- Bestehende Tests müssen nach jedem Task grün bleiben.
- Commits ohne Co-Authored-By-Zeilen.

---

### Task 1: `Client.SetPassword` + `NO_CREDENTIALS`-Marker

**Files:**

- Modify: `internal/zosmf/client.go` (Struct Zeile 18-23, Funktion `Do` Zeile 107-122)
- Test: `internal/zosmf/client_auth_test.go` (neu)

**Interfaces:**

- Consumes: bestehendes `config.AuthConfig` (Felder `Type`, `Username`, `Password`).
- Produces: `func (c *Client) SetPassword(password string)` — thread-safe; `Do` liefert Fehler mit Marker `NO_CREDENTIALS(<SYSTEM>):` wenn `auth.Type == "basic"` und Passwort leer. Task 4 ruft `SetPassword` auf.

- [ ] **Step 1: Failing Tests schreiben**

Neue Datei `internal/zosmf/client_auth_test.go`:

```go
package zosmf

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/config"
)

// newBasicAuthTestClient returns a Client with basic auth against the given
// httptest server. Password may be empty to simulate missing credentials.
func newBasicAuthTestClient(server *httptest.Server, password string) *Client {
	return &Client{
		httpClient: server.Client(),
		baseURL:    server.URL,
		systemName: "sysa",
		auth:       config.AuthConfig{Type: "basic", Username: "user1", Password: password},
	}
}

func TestDo_BasicAuthWithoutPassword_ReturnsNoCredentialsMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request must not reach the server without credentials")
	}))
	defer srv.Close()
	c := newBasicAuthTestClient(srv, "")

	_, err := c.Get("/zosmf/info")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "NO_CREDENTIALS(SYSA):") {
		t.Errorf("error %q does not contain NO_CREDENTIALS(SYSA):", err.Error())
	}
}

func TestSetPassword_SubsequentRequestUsesNewPassword(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	c := newBasicAuthTestClient(srv, "")

	c.SetPassword("geheim")

	if _, err := c.Get("/zosmf/info"); err != nil {
		t.Fatalf("Get after SetPassword: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user1:geheim"))
	if capturedAuth != want {
		t.Errorf("Authorization = %q, want %q", capturedAuth, want)
	}
}

func TestDo_EmptyAuthType_SkipsCredentialCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	c := newTestClient(srv) // auth leer — bestehender Helper aus datasets_test.go

	if _, err := c.Get("/zosmf/info"); err != nil {
		t.Errorf("Get with empty auth: %v", err)
	}
}
```

- [ ] **Step 2: Tests laufen lassen, Fehlschlag verifizieren**

Run: `go test ./internal/zosmf/ -run 'TestDo_BasicAuth|TestSetPassword|TestDo_EmptyAuth' -v`
Expected: FAIL — `c.SetPassword undefined` (Kompilierfehler).

- [ ] **Step 3: Implementierung in `internal/zosmf/client.go`**

Import `sync` ergänzen. Struct erweitern (Zeile 18-23):

```go
// Client is an HTTP client for communicating with a z/OSMF instance.
type Client struct {
	httpClient *http.Client
	baseURL    string
	mu         sync.RWMutex
	auth       config.AuthConfig
	systemName string
}
```

Setter hinter dem Struct einfügen:

```go
// SetPassword replaces the password used for basic auth. It is safe to call
// concurrently with in-flight requests. The password is kept in memory only.
func (c *Client) SetPassword(password string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.auth.Password = password
}
```

`Do` ersetzen (bisher Zeile 107-122):

```go
// Do executes an HTTP request against the z/OSMF instance.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	// Set common z/OSMF headers
	req.Header.Set("X-CSRF-ZOSMF-HEADER", "*")
	req.Header.Set("Accept", "application/json")

	if req.Body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	c.mu.RLock()
	auth := c.auth
	c.mu.RUnlock()

	// Set basic auth if configured
	if auth.Type == "basic" {
		if auth.Password == "" {
			return nil, fmt.Errorf("NO_CREDENTIALS(%s): no password set for this system, set one with the zosmf_set_password tool", strings.ToUpper(c.systemName))
		}
		req.SetBasicAuth(auth.Username, auth.Password)
	}

	return c.httpClient.Do(req)
}
```

- [ ] **Step 4: Tests laufen lassen**

Run: `make test`
Expected: PASS (alle Pakete).

- [ ] **Step 5: Commit**

```bash
git add internal/zosmf/client.go internal/zosmf/client_auth_test.go
git commit -m "feat(auth): add Client.SetPassword and NO_CREDENTIALS marker"
```

---

### Task 2: `AUTH_FAILED`-Marker bei HTTP 401

**Files:**

- Modify: `internal/zosmf/client.go` — Statuscode-Checks in `Get` (Zeile 143-145), `Delete` (Zeile 179-181), `doWithBodyAndStatus` (Zeile 236-238), `doWithBody` (Zeile 266-268)
- Test: `internal/zosmf/client_auth_test.go` (erweitern)

**Interfaces:**

- Consumes: `newBasicAuthTestClient` aus Task 1.
- Produces: Fehler von `Get`/`Put`/`Post`/`Delete` enthalten `AUTH_FAILED(<SYSTEM>):` bei Status 401; alle anderen Status behalten das Format `z/OSMF <path> returned <status>: <body>`.

- [ ] **Step 1: Failing Tests anhängen an `client_auth_test.go`**

```go
func TestGet_401_ReturnsAuthFailedMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := newBasicAuthTestClient(srv, "falsch")

	_, err := c.Get("/zosmf/info")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "AUTH_FAILED(SYSA):") {
		t.Errorf("error %q does not contain AUTH_FAILED(SYSA):", err.Error())
	}
}

func TestGet_500_KeepsGenericErrorFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := newBasicAuthTestClient(srv, "pw")

	_, err := c.Get("/zosmf/info")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), "AUTH_FAILED") {
		t.Errorf("500 must not produce AUTH_FAILED marker: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "returned 500") {
		t.Errorf("error %q does not contain generic status message", err.Error())
	}
}
```

- [ ] **Step 2: Tests laufen lassen, Fehlschlag verifizieren**

Run: `go test ./internal/zosmf/ -run 'TestGet_401|TestGet_500' -v`
Expected: `TestGet_401_ReturnsAuthFailedMarker` FAIL (kein Marker), `TestGet_500_KeepsGenericErrorFormat` PASS.

- [ ] **Step 3: Helper einführen und vier Stellen ersetzen**

In `internal/zosmf/client.go` hinter `Do` einfügen:

```go
// httpError converts a non-2xx z/OSMF response into an error. 401 is marked
// with the AUTH_FAILED marker so clients can react to wrong credentials.
func (c *Client) httpError(path string, status int, body []byte) error {
	if status == http.StatusUnauthorized {
		return fmt.Errorf("AUTH_FAILED(%s): z/OSMF %s returned 401: %s", strings.ToUpper(c.systemName), path, string(body))
	}
	return fmt.Errorf("z/OSMF %s returned %d: %s", path, status, string(body))
}
```

Die vier bestehenden Statuscode-Checks ersetzen — jeweils nur die `return`-Zeile im if-Block:

In `Get` (Zeile 143-145):

```go
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, c.httpError(path, resp.StatusCode, body)
	}
```

In `Delete` (Zeile 179-181):

```go
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, c.httpError(path, resp.StatusCode, respBody)
	}
```

In `doWithBodyAndStatus` (Zeile 236-238):

```go
	if resp.StatusCode < 200 || (resp.StatusCode >= 300 && resp.StatusCode != 202) {
		return resp.StatusCode, nil, c.httpError(path, resp.StatusCode, respBody)
	}
```

In `doWithBody` (Zeile 266-268):

```go
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, c.httpError(path, resp.StatusCode, respBody)
	}
```

`GetWithStatus` nicht anfassen — liefert den Status bewusst an den Aufrufer (Async-Polling).

- [ ] **Step 4: Tests laufen lassen**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/zosmf/client.go internal/zosmf/client_auth_test.go
git commit -m "feat(auth): mark 401 responses with AUTH_FAILED"
```

---

### Task 3: Config — basic auth ohne Passwort zulassen

**Files:**

- Modify: `internal/config/config.go` — `validate()`, case `"basic"`
- Test: `internal/config/config_test.go` (neu)

**Interfaces:**

- Consumes: `config.Load(path string) (*Config, error)` (bestehend).
- Produces: `Load` akzeptiert Systeme mit `auth.username` ohne Passwort (wenn `ZOSMF_PASSWORD` nicht gesetzt); `Auth.Type` wird weiterhin auf `"basic"` normalisiert. Env-Fallback-Verhalten unverändert.

- [ ] **Step 1: Failing Tests schreiben**

Neue Datei `internal/config/config_test.go`:

```go
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
```

- [ ] **Step 2: Tests laufen lassen, Fehlschlag verifizieren**

Run: `go test ./internal/config/ -v`
Expected: `TestLoad_BasicAuthWithoutPassword_Succeeds` FAIL mit „basic auth requires username and password"; die anderen beiden PASS.

- [ ] **Step 3: `validate()` anpassen**

In `internal/config/config.go`, case `"basic"` ersetzen:

```go
		case "basic":
			if sys.Auth.Username == "" {
				return fmt.Errorf("system %s: basic auth requires username", sys.Name)
			}
```

(Passwort ist absichtlich optional: es kann zur Laufzeit per `zosmf_set_password` gesetzt werden; ohne Passwort liefert der Client `NO_CREDENTIALS`.)

- [ ] **Step 4: Tests laufen lassen**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): allow basic auth systems without password"
```

---

### Task 4: Tool `zosmf_set_password` + Registrierung + README

**Files:**

- Create: `internal/tools/auth.go`
- Modify: `internal/tools/registry.go` (handlers-Liste, Zeile 12-55)
- Modify: `README.md` (Tool-Tabelle bei Zeile ~786)
- Test: `internal/tools/auth_test.go` (neu)

**Interfaces:**

- Consumes: `Client.SetPassword` aus Task 1; `zosmf.ClientRegistry.Get` (bestehend); `mcp.ToolHandler`, `mcp.TextResult`, `mcp.ErrorResult`, `mustJSON` (bestehend, `mustJSON` liegt in `internal/tools/jobs.go`).
- Produces: MCP-Tool `zosmf_set_password` mit Argumenten `{ system: string, password: string }` (beide required). Erfolg: `TextResult` ohne Passwort im Text. Unbekanntes System: `ErrorResult` mit `unknown system: <name>`.

- [ ] **Step 1: Failing Tests schreiben**

Neue Datei `internal/tools/auth_test.go`:

```go
package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/config"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

func TestSetPasswordTool_UpdatesClientPassword(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	client, err := zosmf.NewClient(config.SystemConfig{
		Name: "SYSA",
		Host: srv.URL,
		Auth: config.AuthConfig{Type: "basic", Username: "user1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := zosmf.NewClientRegistry()
	registry.Register("SYSA", client)

	tool := NewSetPasswordTool(registry)
	args, _ := json.Marshal(map[string]string{"system": "SYSA", "password": "geheim"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned error result: %+v", result.Content)
	}
	if strings.Contains(result.Content[0].Text, "geheim") {
		t.Error("result text must not contain the password")
	}

	if _, err := client.Get("/zosmf/info"); err != nil {
		t.Fatalf("Get after set_password: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user1:geheim"))
	if capturedAuth != want {
		t.Errorf("Authorization = %q, want %q", capturedAuth, want)
	}
}

func TestSetPasswordTool_UnknownSystem_ReturnsErrorResult(t *testing.T) {
	tool := NewSetPasswordTool(zosmf.NewClientRegistry())
	args, _ := json.Marshal(map[string]string{"system": "NOPE", "password": "x"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for unknown system")
	}
	if !strings.Contains(result.Content[0].Text, "unknown system") {
		t.Errorf("unexpected error text: %q", result.Content[0].Text)
	}
}

func TestSetPasswordTool_MissingArgs_ReturnsErrorResult(t *testing.T) {
	tool := NewSetPasswordTool(zosmf.NewClientRegistry())
	args, _ := json.Marshal(map[string]string{"system": "SYSA"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing password")
	}
}
```

- [ ] **Step 2: Tests laufen lassen, Fehlschlag verifizieren**

Run: `go test ./internal/tools/ -run TestSetPasswordTool -v`
Expected: FAIL — `NewSetPasswordTool undefined` (Kompilierfehler).

- [ ] **Step 3: Tool implementieren**

Neue Datei `internal/tools/auth.go`:

```go
package tools

import (
	"context"
	"encoding/json"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

// --- Set Password ---

type setPasswordTool struct {
	clients *zosmf.ClientRegistry
}

type setPasswordArgs struct {
	System   string `json:"system"`
	Password string `json:"password"`
}

func NewSetPasswordTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &setPasswordTool{clients: clients}
}

func (t *setPasswordTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_set_password",
		Description: "Set the z/OSMF password for a configured system at runtime. " +
			"The password is kept in memory only and is used for all subsequent " +
			"requests to that system. Use this when a tool call failed with a " +
			"NO_CREDENTIALS or AUTH_FAILED error.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{
					"type":        "string",
					"description": "Name of the z/OS system",
				},
				"password": map[string]any{
					"type":        "string",
					"description": "Password for the system's configured username",
				},
			},
			"required": []string{"system", "password"},
		}),
	}
}

func (t *setPasswordTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args setPasswordArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	if args.System == "" || args.Password == "" {
		return mcp.ErrorResult("system and password are required"), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	client.SetPassword(args.Password)
	return mcp.TextResult("password updated for system " + args.System), nil
}
```

In `internal/tools/registry.go` in der `handlers`-Liste vor `// Jobs tools` einfügen:

```go
		// Auth tools
		NewSetPasswordTool(clients),
```

- [ ] **Step 4: Tests laufen lassen**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: README-Tooltabelle ergänzen**

In `README.md`, Tool-Tabelle (bei Zeile ~786, dort steht `| zosmf_issue_command | Issue an operator command |`) eine Zeile ergänzen:

```markdown
| `zosmf_set_password`     | Set the password for a system at runtime (in-memory only) |
```

- [ ] **Step 6: Commit**

```bash
git add internal/tools/auth.go internal/tools/auth_test.go internal/tools/registry.go README.md
git commit -m "feat(auth): add zosmf_set_password tool"
```

---

### Task 5: Gesamtverifikation

**Files:**

- Keine Änderungen — nur Verifikation.

**Interfaces:**

- Consumes: alle vorherigen Tasks.
- Produces: grüner Build + Testlauf auf dem Branch.

- [ ] **Step 1: Kompletter Durchlauf**

Run: `make all`
Expected: fmt, vet, test, build-local — alles ohne Fehler.

- [ ] **Step 2: Smoke-Test über stdio**

```bash
{ echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}'; \
  echo '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'; } | ./bin/zosmf-mcp --transport stdio --config samp/config.yaml 2>/dev/null | grep -o 'zosmf_set_password' | head -1
```

Expected: Ausgabe `zosmf_set_password` (Tool erscheint in tools/list). Falls `samp/config.yaml` nicht existiert: beliebige valide Config verwenden.

- [ ] **Step 3: Manuelle Verifikation gegen echtes z/OSMF (optional, benötigt Host-Zugang)**

Config mit System ohne Passwort, `ZOSMF_PASSWORD` nicht gesetzt: Tool-Call (z.B. `zosmf_list_jobs`) → Fehlertext enthält `NO_CREDENTIALS(<SYSTEM>):`. Danach `zosmf_set_password` mit korrektem Passwort → gleicher Tool-Call liefert Daten. Mit falschem Passwort → `AUTH_FAILED(<SYSTEM>):`. **Achtung:** falsches Passwort nur einmal testen, dann korrektes setzen — wiederholte Fehlversuche führen zum RACF-Revoke.
