# Einheitliche Config-Datei Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** zosmf-mcp findet `.smpe-zosmf.yaml` ohne `--config` selbst (Legacy-Datei mit Deprecation-Warnung), toleriert Alt-Einträge ohne `user`, und alle vier Repos dokumentieren dasselbe kanonische `servers:`-Schema.

**Architecture:** Eine pure, testbare Funktion `config.ResolveDefaultPath(cwd, home)` liefert den Default-Pfad; `main.go` ruft sie nur bei leerem `--config`. Einträge ohne Auth werden beim Laden toleriert und per `missingUsername`-Flag im Client markiert — Tool-Calls darauf liefern den bestehenden `NO_CREDENTIALS`-Marker mit „no username"-Text, ohne HTTP-Request. Alles Weitere ist README-/Template-Pflege in vier Repos.

**Tech Stack:** Go (zosmf_mcp), Markdown/TS-Template/Lua-Template (Doku-Tasks).

**Spec:** `docs/2026-07-11-unified-config-design.md` (zosmf_mcp, committet auf main).

## Global Constraints

- zosmf_mcp: Branch `feat/unified-config` ab `main` (vor Task 1 anlegen). Doku-Tasks in den anderen Repos committen direkt auf deren `main` (reine Doku/Templates) — vor jedem Commit dort `git status` prüfen und nur die eigenen Dateien stagen.
- Suchreihenfolge exakt: (1) `./.smpe-zosmf.yaml`, (2) `~/.config/smpe_ls/.smpe-zosmf.yaml`, (3) `./.zosmf-mcp-config.yaml` mit stderr-Warnung `WARNING: .zosmf-mcp-config.yaml is deprecated — rename to .smpe-zosmf.yaml`, (4) Fehler mit allen drei geprüften Pfaden.
- Explizites `--config <pfad>`: Verhalten byte-identisch zu heute.
- Marker-Format-Vertrag unverändert; neuer Fehlertext für Einträge ohne User: `NO_CREDENTIALS(<SYSTEM>): no username configured — add 'user:' to the server entry` (enthalten, nicht verankert).
- Bestehende Tests müssen grün bleiben — insbesondere `TestDo_EmptyAuthType_SkipsCredentialCheck` (Test-Clients ohne Auth dürfen NICHT als „missing username" gelten; darum Flag statt Type-Check).
- Kanonischer Schlüssel in aller Doku: `servers:`. Passwörter nie in Beispiel-Dateien im Klartext — `${ENV_VAR}` zeigen.
- Builds/Tests via make: zosmf_mcp `make test`/`make all`; vscpdf `make build` (nur falls dort Code — hier nicht). Commits ohne Co-Authored-By-Zeilen.

---

### Task 1: `config.ResolveDefaultPath` + main.go-Verdrahtung (zosmf_mcp)

**Files:**

- Modify: `internal/config/config.go` (neue Funktion am Dateiende)
- Modify: `cmd/zosmf-mcp/main.go:24` (Flag-Default) und nach `flag.Parse()`/Version-Block (Auflösung)
- Test: `internal/config/config_test.go` (anhängen)

**Interfaces:**

- Produces: `func ResolveDefaultPath(cwd, home string) (path string, deprecated bool, err error)` — pur, keine Env-Zugriffe; `main.go` übergibt `os.Getwd()`/`os.UserHomeDir()`.

- [ ] **Step 1: Failing Tests schreiben**

An `internal/config/config_test.go` anhängen:

```go
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
```

(Imports `path/filepath`, `strings` ergänzen, falls nicht vorhanden. Falls der Helper-Name `writeFile` mit Bestehendem kollidiert: `writeEmptyConfig` verwenden — vorher grep.)

- [ ] **Step 2: Fehlschlag verifizieren**

Run: `go test ./internal/config/ -run TestResolveDefaultPath -v`
Expected: FAIL — `undefined: ResolveDefaultPath`.

- [ ] **Step 3: Implementieren**

In `internal/config/config.go`:

```go
// DefaultConfigName is the canonical config shared by smpe_ls, vscpdf,
// nvpdf and zosmf-mcp. LegacyConfigName is kept for backwards compatibility.
const (
	DefaultConfigName = ".smpe-zosmf.yaml"
	LegacyConfigName  = ".zosmf-mcp-config.yaml"
)

// ResolveDefaultPath locates the configuration file when --config is not
// given: cwd, then ~/.config/smpe_ls/, then the deprecated legacy name in
// cwd. deprecated is true when the legacy file was chosen.
func ResolveDefaultPath(cwd, home string) (path string, deprecated bool, err error) {
	candidates := []struct {
		path       string
		deprecated bool
	}{
		{filepath.Join(cwd, DefaultConfigName), false},
		{filepath.Join(home, ".config", "smpe_ls", DefaultConfigName), false},
		{filepath.Join(cwd, LegacyConfigName), true},
	}
	for _, c := range candidates {
		if _, statErr := os.Stat(c.path); statErr == nil {
			return c.path, c.deprecated, nil
		}
	}
	return "", false, fmt.Errorf("no configuration file found; searched: %s, %s, %s",
		candidates[0].path, candidates[1].path, candidates[2].path)
}
```

(Import `path/filepath` ergänzen.)

In `cmd/zosmf-mcp/main.go`: Flag-Default leeren und Auflösung einfügen —

```go
	configPath := flag.String("config", "", "Path to configuration file (default: search .smpe-zosmf.yaml in cwd and ~/.config/smpe_ls/)")
```

Nach dem Version-Block, vor `config.Load`:

```go
	resolvedConfig := *configPath
	if resolvedConfig == "" {
		cwd, err := os.Getwd()
		if err != nil {
			logger.Fatalf("cannot determine working directory: %v", err)
		}
		home, _ := os.UserHomeDir()
		path, deprecated, err := config.ResolveDefaultPath(cwd, home)
		if err != nil {
			logger.Fatalf("%v", err)
		}
		if deprecated {
			logger.Printf("WARNING: %s is deprecated — rename to %s", config.LegacyConfigName, config.DefaultConfigName)
		}
		resolvedConfig = path
	}

	cfg, err := config.Load(resolvedConfig)
```

(Die bisherige Zeile `cfg, err := config.Load(*configPath)` ersetzen; `logger` muss dafür vor die Auflösung wandern, falls er aktuell erst danach initialisiert wird — Reihenfolge im Code prüfen.)

- [ ] **Step 4: Tests laufen lassen**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go cmd/zosmf-mcp/main.go
git commit -m "feat(config): default search for .smpe-zosmf.yaml with legacy fallback"
```

---

### Task 2: Einträge ohne `user` tolerieren (zosmf_mcp)

**Files:**

- Modify: `internal/config/config.go` (`validate()`, case-Verzweigung)
- Modify: `internal/zosmf/client.go` (`Client`-Struct, `NewClient`, `Do`)
- Test: `internal/config/config_test.go`, `internal/zosmf/client_auth_test.go` (anhängen)

**Interfaces:**

- Consumes: bestehendes `NO_CREDENTIALS(<SYSTEM>):`-Marker-Muster in `Do` (client.go).
- Produces: `Client.missingUsername bool` (privat, von `NewClient` gesetzt, wenn `sys.Auth.Type == ""`); `validate()` akzeptiert Einträge ohne Auth.

- [ ] **Step 1: Failing Tests schreiben**

`internal/config/config_test.go`:

```go
func TestLoad_ServerWithoutUser_Tolerated(t *testing.T) {
	t.Setenv("ZOSMF_PASSWORD", "")
	yaml := `
servers:
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
```

`internal/zosmf/client_auth_test.go`:

```go
func TestDo_MissingUsername_ReturnsNoCredentialsMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request must not reach the server without a username")
	}))
	defer srv.Close()

	client, err := NewClient(config.SystemConfig{Name: "LEGACY", Host: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get("/zosmf/info")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "NO_CREDENTIALS(LEGACY):") || !strings.Contains(err.Error(), "no username") {
		t.Errorf("error %q lacks marker/text", err.Error())
	}
}
```

- [ ] **Step 2: Fehlschlag verifizieren**

Run: `go test ./internal/config/ ./internal/zosmf/ -run 'WithoutUser|MissingUsername' -v`
Expected: `TestLoad_ServerWithoutUser_Tolerated` FAIL („auth type must be 'basic' or 'certificate'"); `TestDo_MissingUsername_...` FAIL (kein Fehler oder falscher Text).

- [ ] **Step 3: Implementieren**

`internal/config/config.go`, `validate()`: den `default:`-Zweig des `switch sys.Auth.Type` ersetzen —

```go
		case "":
			// Tolerated: legacy smpe_ls entries without user/auth. The client
			// rejects tool calls with a NO_CREDENTIALS marker instead of
			// blocking server startup for all other systems.
		default:
			return fmt.Errorf("system %s: auth type must be 'basic' or 'certificate', got '%s'", sys.Name, sys.Auth.Type)
```

`internal/zosmf/client.go`: Struct-Feld `missingUsername bool` ergänzen; in `NewClient` vor dem `return`:

```go
	missingUsername := sys.Auth.Type == ""
```

und ins Struct-Literal `missingUsername: missingUsername,` aufnehmen. In `Do`, vor dem bestehenden `if auth.Type == "basic"`-Block:

```go
	if c.missingUsername {
		return nil, fmt.Errorf("NO_CREDENTIALS(%s): no username configured — add 'user:' to the server entry", strings.ToUpper(c.systemName))
	}
```

(Test-Clients, die das Struct direkt bauen, haben `missingUsername == false` — bestehende Tests bleiben grün.)

- [ ] **Step 4: Tests laufen lassen**

Run: `make test`
Expected: PASS, inklusive `TestDo_EmptyAuthType_SkipsCredentialCheck`.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go internal/zosmf/client.go internal/zosmf/client_auth_test.go
git commit -m "feat(config): tolerate server entries without user, reject at call time"
```

---

### Task 3: zosmf_mcp README — kanonisches Schema (Source of Truth)

**Files:**

- Modify: `README.md` (Beispiel Zeile ~77 `systems:` → `servers:`, Konfigurations-Abschnitt, `--config`-Doku)

**Interfaces:** keine.

- [ ] **Step 1: README umstellen**

1. Alle Konfigurations-Beispiele von `systems:` auf `servers:` umstellen (Suche `systems:` im README; `ssh_systems:` NICHT umbenennen).
2. Beim `--config`-Flag die neue Default-Suchreihenfolge dokumentieren (cwd → `~/.config/smpe_ls/` → Legacy mit Warnung) und `.zosmf-mcp-config.yaml` als deprecated markieren.
3. Ein kanonisches Vollbeispiel einfügen (an der Stelle des bisherigen Hauptbeispiels), Hinweis, dass dieselbe Datei von vscode-smpe, vscpdf und nvpdf.nvim gelesen wird und unbekannte Felder dort ignoriert werden:

```yaml
# .smpe-zosmf.yaml — shared by zosmf-mcp, vscode-smpe, vscpdf and nvpdf.nvim
servers:
  - name: PROD
    host: https://zosmf.example.com
    port: 443
    user: USER1                  # z/OSMF userid (basic auth)
    rejectUnauthorized: true
    # auth:                      # alternative to user:, with env expansion
    #   username: USER1
    #   password: ${PROD_PASSWORD}
    # tso_account: ACCT1         # required for zosmf_tso_command
    # tso_proc: IKJACCNT
    # csi:                       # SMP/E (used by vscode-smpe and zosmf_smpe_query_csi)
    #   - SMPE.GLOBAL.CSI
    # zones: [GLOBAL, TGT1]
    # defaultZones: [GLOBAL]
    # home: /u/user1             # USS home (used by vscpdf/nvpdf)

# ssh_systems:                   # optional, for ZOAU tools via SSH
#   - name: PROD
#     host: prod.example.com
#     port: 22
#     username: user1
#     key_file: ~/.ssh/id_rsa

# defaultServer: PROD            # used by vscode-smpe; ignored by zosmf-mcp
```

- [ ] **Step 2: Verifizieren und committen**

Run: `grep -n "systems:" README.md | grep -v ssh_systems`
Expected: keine Treffer.

```bash
git add README.md
git commit -m "docs: canonical .smpe-zosmf.yaml schema with servers key"
```

---

### Task 4: Client-Templates und Doku (vscode-smpe, nvpdf.nvim, vscpdf)

**Files:**

- Modify: `~/src/smpe_ls/client/vscode-smpe/src/zosmf/configManager.ts` (`CONFIG_TEMPLATE`, Zeile ~18)
- Modify: `~/src/nvpdf.nvim/lua/nvpdf/template.lua` (`M.TEMPLATE`)
- Modify: `~/src/vscpdf/README.md:65` und `~/src/vscpdf/CLAUDE.md:83` (`systems:` → `servers:` im Beispiel)

**Interfaces:** keine — reine Doku/Template-Pflege, KEINE Logik-Änderungen in diesen Repos.

- [ ] **Step 1: vscode-smpe-Template erweitern**

Im eingebetteten `CONFIG_TEMPLATE` (String-Literal) nach den bestehenden Feldern des ersten Server-Eintrags dieselben auskommentierten optionalen Felder ergänzen wie im zosmf_mcp-README-Vollbeispiel aus Task 3 (`auth:` mit `${ENV}`-Hinweis, `tso_account`, `tso_proc`, `home`) und am Template-Ende den auskommentierten `ssh_systems:`-Block plus eine Kopfzeile `# Shared with zosmf-mcp, vscpdf and nvpdf.nvim — unknown fields are ignored per component.` Nur den Template-String ändern, keinen Code.

Run (im Repo smpe_ls): `cd client/vscode-smpe && npm run compile` (bzw. das dortige Build-Skript aus package.json — vorher nachsehen) → exit 0.

```bash
git -C ~/src/smpe_ls add client/vscode-smpe/src/zosmf/configManager.ts
git -C ~/src/smpe_ls commit -m "docs: document shared optional config fields in template"
```

- [ ] **Step 2: nvpdf-Template erweitern**

In `lua/nvpdf/template.lua` (`M.TEMPLATE`-String): dieselben auskommentierten Felder/Blöcke wie in Step 1 ergänzen (gleiche Texte). Nur der String ändert sich.

Run: `nvim --headless -c "lua require('nvpdf.template')" -c q` (Syntax-Check; falls nvim fehlt: `luajit -e "dofile('~/src/nvpdf.nvim/lua/nvpdf/template.lua')"` — schlägt wegen `vim`-Global evtl. fehl, dann reicht Sichtprüfung der String-Syntax).

```bash
git -C ~/src/nvpdf.nvim add lua/nvpdf/template.lua
git -C ~/src/nvpdf.nvim commit -m "docs: document shared optional config fields in template"
```

- [ ] **Step 3: vscpdf-Beispiele vereinheitlichen**

In `README.md` (~Zeile 65) und `CLAUDE.md` (~Zeile 83) den Beispiel-Schlüssel `systems:` durch `servers:` ersetzen (nur das Beispiel; Text drumherum anpassen falls er „systems" wörtlich nennt). ACHTUNG: `CLAUDE.md` ist in vscpdf gitignored — nur auf Disk ändern, nicht committen; `README.md` normal committen.

```bash
git -C ~/src/vscpdf add README.md
git -C ~/src/vscpdf commit -m "docs: use canonical servers key in config example"
```

---

### Task 5: Gesamtverifikation (zosmf_mcp)

**Files:** keine Änderungen.

- [ ] **Step 1: Voller Durchlauf**

Run (in zosmf_mcp): `make all`
Expected: fmt, vet, test (-race), build — alles grün.

- [ ] **Step 2: Smoke — Default-Suche**

```bash
cd "$(mktemp -d)" && printf 'servers:\n  - name: SYSA\n    host: https://zosmf.example.com\n    user: USER1\n' > .smpe-zosmf.yaml
{ echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}'; } | ~/src/zosmf_mcp/bin/zosmf-mcp --transport stdio 2>&1 | grep -m1 "registered system: SYSA"
```

Expected: Zeile `registered system: SYSA` (Server fand die Datei ohne `--config`).

- [ ] **Step 3: Smoke — Legacy-Warnung**

Im selben Temp-Verzeichnis: `mv .smpe-zosmf.yaml .zosmf-mcp-config.yaml`, gleichen Aufruf wiederholen, zusätzlich `grep -m1 "deprecated"`.
Expected: sowohl `registered system: SYSA` als auch die Deprecation-Warnung.

- [ ] **Step 4: Bestands-Check (manuell, optional mit echter Kunden-Datei)**

Eine unveränderte `.smpe-zosmf.yaml` mit nur smpe_ls-Feldern (`servers:`, `csi`, `zones`, `user`) gegen den neuen Server: Start ok; Eintrag ohne `user` blockiert den Start nicht (nur `NO_CREDENTIALS` beim Call); vscpdf/nvpdf funktionieren unverändert (keine Code-Änderung dort).
