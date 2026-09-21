# Guided Setup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `zosmf-mcp` a setup that produces the same configuration whoever runs it — a human at a terminal, Claude Code, or an agent driving a small local model.

**Architecture:** Two new subcommands (`init config`, `init env`) do everything that must be deterministic. One field catalog in Go feeds the wizard prompts, the flag set and a JSON question list, so all three stay in step. The YAML is edited on the node level, never re-serialised from structs, because the file is shared with `smpe_ls`, vscode-smpe and nvpdf. A short instruction file under `agents/` drives the commands and covers what the binary cannot reach.

**Tech Stack:** Go 1.26, `gopkg.in/yaml.v3`, standard library `flag` with `flag.NewFlagSet` per subcommand. No new dependencies.

**Spec:** `docs/2026-09-21-guided-setup-design.md`

## Global Constraints

- Go 1.26; the module has exactly two dependencies (`github.com/google/uuid`, `gopkg.in/yaml.v3`) and this work adds none.
- Subcommands are parsed by hand with `flag.NewFlagSet` — no CLI framework.
- `make fmt vet lint test` passes before every commit; lint is golangci-lint `v2.12.0` via `make lint`.
- `make check-identifiers` passes: documentation examples use `example.com` (RFC 2606) and `192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24` (RFC 5737). No home directory paths naming a real user.
- Tool and CLI error strings are a public interface. New strings are fine; existing ones are not reworded.
- No password, bearer token or API key is ever printed, logged, or written into a YAML file.
- Tests that go through `config.Load` clear the ambient environment with the existing `unsetEnv(t, …)` helper in `internal/config/config_test.go`.
- Files stay focused: no file in `internal/setup` grows past roughly 250 lines; split by responsibility instead.

---

## File Structure

| File | Responsibility |
| --- | --- |
| `internal/config/password.go` (create) | `PasswordEnvVar(system)` — the one place the convention lives |
| `internal/config/config.go` (modify) | password lookup order in `normalize`, records the source |
| `internal/setup/fields.go` (create) | the field catalog: one list, plus the JSON encoder |
| `internal/setup/values.go` (create) | collected answers, independent of flags or prompts |
| `internal/setup/flags.go` (create) | binds the catalog to a `flag.FlagSet`, validates, errors naming flags |
| `internal/setup/document.go` (create) | `yaml.Node` load, upsert, render, backup |
| `internal/setup/prompt.go` (create) | the TTY wizard |
| `internal/setup/env.go` (create) | `.env` reading and writing |
| `cmd/zosmf-mcp/init.go` (create) | subcommand dispatch and the two command bodies |
| `cmd/zosmf-mcp/main.go` (modify) | hand `init` off before the server starts |
| `agents/setup.md` (create) | the canonical procedure |
| `agents/claude/SKILL.md` (create) | Claude Code wrapper |
| `Makefile` (modify) | `install-skill` target |
| `README.md`, `CHANGELOG.md` (modify) | getting started, release notes |

---

## Task 1: Per-system password convention

**Files:**
- Create: `internal/config/password.go`
- Modify: `internal/config/config.go` (the `normalize` password block, around L182-L195)
- Test: `internal/config/password_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `config.PasswordEnvVar(system string) string`, and a new field `SystemConfig.PasswordSource string` (values: `"file"`, `"system-env"`, `"global-env"`, `"none"`) that `cmd` reads for its startup log.

- [ ] **Step 1: Write the failing test for the variable name**

Create `internal/config/password_test.go`:

```go
package config

import "testing"

func TestPasswordEnvVar(t *testing.T) {
	for _, tc := range []struct{ system, want string }{
		{"MAINT", "ZOSMF_MAINT_PASSWORD"},
		{"maint", "ZOSMF_MAINT_PASSWORD"},
		{"PROD-A", "ZOSMF_PROD_A_PASSWORD"},
		{"sys.1", "ZOSMF_SYS_1_PASSWORD"},
	} {
		if got := PasswordEnvVar(tc.system); got != tc.want {
			t.Errorf("PasswordEnvVar(%q) = %q, want %q", tc.system, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run TestPasswordEnvVar ./internal/config/`
Expected: FAIL, `undefined: PasswordEnvVar`

- [ ] **Step 3: Implement the helper**

Create `internal/config/password.go`:

```go
package config

import "strings"

// PasswordEnvVar is the environment variable a system's password is looked
// up under when the configuration file names none. The name derives from the
// system name so there is nothing to keep in sync by hand: uppercased, with
// every character outside A-Z and 0-9 replaced by an underscore.
func PasswordEnvVar(system string) string {
	var b strings.Builder
	b.WriteString("ZOSMF_")
	for _, r := range strings.ToUpper(system) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	b.WriteString("_PASSWORD")
	return b.String()
}
```

- [ ] **Step 4: Run it and watch it pass**

Run: `go test -run TestPasswordEnvVar ./internal/config/`
Expected: PASS

- [ ] **Step 5: Write the failing tests for the lookup order**

Append to `internal/config/password_test.go`:

```go
const yamlTwoSystems = `
servers:
  - name: MAINT
    host: https://zos1.example.com:443
    user: IBMUSER
  - name: PROD
    host: https://zos2.example.com:443
    user: IBMUSER
`

func TestSystemEnvPasswordWins(t *testing.T) {
	unsetEnv(t, "ZOSMF_PASSWORD")
	t.Setenv("ZOSMF_MAINT_PASSWORD", "from-system-var")

	cfg := loadTestConfig(t, yamlTwoSystems)

	if got := cfg.Systems[0].Auth.Password; got != "from-system-var" {
		t.Errorf("MAINT password = %q, want from-system-var", got)
	}
	if got := cfg.Systems[0].PasswordSource; got != "system-env" {
		t.Errorf("MAINT source = %q, want system-env", got)
	}
	if got := cfg.Systems[1].Auth.Password; got != "" {
		t.Errorf("PROD password = %q, want empty — the variable is MAINT's", got)
	}
	if got := cfg.Systems[1].PasswordSource; got != "none" {
		t.Errorf("PROD source = %q, want none", got)
	}
}

func TestGlobalEnvPasswordStillApplies(t *testing.T) {
	unsetEnv(t, "ZOSMF_MAINT_PASSWORD")
	t.Setenv("ZOSMF_PASSWORD", "from-global-var")

	cfg := loadTestConfig(t, yamlTwoSystems)

	for i, sys := range cfg.Systems {
		if sys.Auth.Password != "from-global-var" {
			t.Errorf("system %d password = %q, want from-global-var", i, sys.Auth.Password)
		}
		if sys.PasswordSource != "global-env" {
			t.Errorf("system %d source = %q, want global-env", i, sys.PasswordSource)
		}
	}
}

// A system-specific variable beats the global one; both lose to the file.
func TestFilePasswordBeatsBothVariables(t *testing.T) {
	t.Setenv("ZOSMF_PASSWORD", "from-global-var")
	t.Setenv("ZOSMF_MAINT_PASSWORD", "from-system-var")

	cfg := loadTestConfig(t, `
servers:
  - name: MAINT
    host: https://zos1.example.com:443
    user: IBMUSER
    auth:
      password: in-the-file
`)

	if got := cfg.Systems[0].Auth.Password; got != "in-the-file" {
		t.Errorf("password = %q, want in-the-file", got)
	}
	if got := cfg.Systems[0].PasswordSource; got != "file" {
		t.Errorf("source = %q, want file", got)
	}
}
```

- [ ] **Step 6: Run them and watch them fail**

Run: `go test -run 'TestSystemEnvPassword|TestGlobalEnvPassword|TestFilePassword' ./internal/config/`
Expected: FAIL — `cfg.Systems[0].PasswordSource` undefined, and MAINT has no password.

- [ ] **Step 7: Add the field**

In `internal/config/config.go`, in `SystemConfig` (after `Proxy`, before the alias block):

```go
	// PasswordSource records where the password came from: "file",
	// "system-env", "global-env" or "none". Set by normalize, logged at
	// startup, never serialised.
	PasswordSource string `yaml:"-"`
```

- [ ] **Step 8: Replace the password block in normalize**

In `internal/config/config.go`, replace:

```go
		// password from env if not in config
		if s.Auth.Password == "" && envPassword != "" {
			s.Auth.Password = envPassword
		}
```

with:

```go
		// Password sources, in order: the file, a per-system variable, the
		// global variable. The per-system name derives from the system name,
		// so nothing has to be kept in sync between file and environment.
		switch {
		case s.Auth.Password != "":
			s.PasswordSource = "file"
		default:
			if v := os.Getenv(PasswordEnvVar(s.Name)); v != "" {
				s.Auth.Password = v
				s.PasswordSource = "system-env"
			} else if envPassword != "" {
				s.Auth.Password = envPassword
				s.PasswordSource = "global-env"
			} else {
				s.PasswordSource = "none"
			}
		}
```

- [ ] **Step 9: Run them and watch them pass**

Run: `env -u MCP_API_KEY go test -count=1 ./internal/config/`
Expected: PASS, including the existing tests.

- [ ] **Step 10: Log the source at startup**

In `cmd/zosmf-mcp/main.go`, in the system registration loop, after the existing `registered system:` line:

```go
		logger.Printf("system %s: password from %s", sys.Name, sys.PasswordSource)
```

- [ ] **Step 11: Verify the whole suite and the binary**

Run: `make fmt && make vet && env -u MCP_API_KEY make test && make lint`
Expected: all green, lint reports `0 issues`.

- [ ] **Step 12: Commit**

```bash
git add internal/config/password.go internal/config/password_test.go internal/config/config.go cmd/zosmf-mcp/main.go
git commit -m "feat(config): look up a per-system password by convention

A system-specific password used to require naming the variable in the
file. The name now derives from the system name, and the server logs
which of the three sources a password came from."
```

---

## Task 2: The field catalog

**Files:**
- Create: `internal/setup/fields.go`, `internal/setup/values.go`
- Test: `internal/setup/fields_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `setup.Field` with exported fields `ID, YAMLKey, Label, Help, Kind, Cardinality, Required, Default, Example, Group string`-ish (exact types below)
  - `setup.ConfigFields() []Field`
  - `setup.CatalogJSON(command string, fields []Field) ([]byte, error)`
  - `setup.Values` with `Set(id string, vals []string)`, `One(id) string`, `Many(id) []string`, `Has(id) bool`

- [ ] **Step 1: Write the failing test**

Create `internal/setup/fields_test.go`:

```go
package setup

import (
	"encoding/json"
	"testing"
)

// The catalog is the contract three consumers share: wizard, flags and the
// JSON output. A field that is missing an id or a help text breaks the
// weakest of them — an agent that has nothing else to go on.
func TestConfigFieldsAreComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range ConfigFields() {
		if f.ID == "" || f.Label == "" || f.Help == "" || f.Group == "" {
			t.Errorf("incomplete field: %+v", f)
		}
		if f.Kind != KindString && f.Kind != KindInt && f.Kind != KindBool {
			t.Errorf("field %s has unknown kind %q", f.ID, f.Kind)
		}
		if f.Cardinality != One && f.Cardinality != Many {
			t.Errorf("field %s has unknown cardinality %q", f.ID, f.Cardinality)
		}
		if seen[f.ID] {
			t.Errorf("duplicate field id %q", f.ID)
		}
		seen[f.ID] = true
	}
	for _, required := range []string{"name", "host", "user", "csi", "zones"} {
		if !seen[required] {
			t.Errorf("catalog is missing %q", required)
		}
	}
}

func TestCSIAndZonesTakeLists(t *testing.T) {
	for _, f := range ConfigFields() {
		if (f.ID == "csi" || f.ID == "zones") && f.Cardinality != Many {
			t.Errorf("%s must be a list, got %q", f.ID, f.Cardinality)
		}
	}
}

func TestCatalogJSONShape(t *testing.T) {
	data, err := CatalogJSON("init config", ConfigFields())
	if err != nil {
		t.Fatalf("CatalogJSON: %v", err)
	}
	var got struct {
		Command string `json:"command"`
		Fields  []struct {
			ID          string `json:"id"`
			Cardinality string `json:"cardinality"`
			Help        string `json:"help"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got.Command != "init config" {
		t.Errorf("command = %q", got.Command)
	}
	if len(got.Fields) != len(ConfigFields()) {
		t.Errorf("got %d fields, want %d", len(got.Fields), len(ConfigFields()))
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./internal/setup/`
Expected: FAIL, the package does not exist yet.

- [ ] **Step 3: Write the catalog**

Create `internal/setup/fields.go`:

```go
// Package setup builds and edits the configuration files that zosmf-mcp
// reads. One field catalog feeds the interactive wizard, the command-line
// flags and the JSON question list, so the three cannot drift apart.
package setup

import "encoding/json"

type Kind string

const (
	KindString Kind = "string"
	KindInt    Kind = "int"
	KindBool   Kind = "bool"
)

type Cardinality string

const (
	One  Cardinality = "one"
	Many Cardinality = "many"
)

// Field describes one answer the setup needs. ID is the flag name without
// dashes; YAMLKey is where the answer goes in the configuration file.
type Field struct {
	ID          string      `json:"id"`
	YAMLKey     string      `json:"yaml_key"`
	Label       string      `json:"label"`
	Help        string      `json:"help"`
	Kind        Kind        `json:"type"`
	Cardinality Cardinality `json:"cardinality"`
	Required    bool        `json:"required"`
	Default     string      `json:"default,omitempty"`
	Example     any         `json:"example,omitempty"`
	Group       string      `json:"group"`
}

// ConfigFields is the catalog for `init config`.
func ConfigFields() []Field {
	return []Field{
		{
			ID: "name", YAMLKey: "name", Label: "System name",
			Help:  "Name used as the 'system' argument in tool calls. Uppercased.",
			Kind:  KindString, Cardinality: One, Required: true,
			Example: "MAINT", Group: "system",
		},
		{
			ID: "host", YAMLKey: "host", Label: "z/OSMF base URL",
			Help:  "Including the scheme, for example https://zos.example.com:443",
			Kind:  KindString, Cardinality: One, Required: true,
			Example: "https://zos.example.com:443", Group: "system",
		},
		{
			ID: "port", YAMLKey: "port", Label: "Port",
			Help:  "Only needed when the URL carries no port.",
			Kind:  KindInt, Cardinality: One, Group: "system", Example: 443,
		},
		{
			ID: "user", YAMLKey: "user", Label: "TSO user id",
			Help:  "The z/OSMF user. Its password belongs in the environment, never here.",
			Kind:  KindString, Cardinality: One, Required: true,
			Example: "IBMUSER", Group: "system",
		},
		{
			ID: "csi", YAMLKey: "csi", Label: "CSI datasets",
			Help:  "SMP/E tools use the first entry when a call names no csi_dataset.",
			Kind:  KindString, Cardinality: Many,
			Example: []string{"ZOS31.SMPE.GLOBAL.CSI"}, Group: "system",
		},
		{
			ID: "zones", YAMLKey: "zones", Label: "SMP/E zones",
			Help:  "All entries are the default when a call names no zone.",
			Kind:  KindString, Cardinality: Many,
			Example: []string{"MVST100", "MVSD100"}, Group: "system",
		},
		{
			ID: "default-zones", YAMLKey: "defaultZones", Label: "Default zones for vscode-smpe",
			Help:  "Read by vscode-smpe, ignored by this server. Usually the same as zones.",
			Kind:  KindString, Cardinality: Many, Group: "system",
		},
		{
			ID: "tso-account", YAMLKey: "tso_account", Label: "TSO account number",
			Help:  "zosmf_tso_command fails without it.",
			Kind:  KindString, Cardinality: One, Example: "ACCT#", Group: "system",
		},
		{
			ID: "tso-proc", YAMLKey: "tso_proc", Label: "TSO logon procedure",
			Help:  "zosmf_tso_command fails without it.",
			Kind:  KindString, Cardinality: One, Example: "IZUFPROC", Group: "system",
		},
		{
			ID: "reject-unauthorized", YAMLKey: "rejectUnauthorized", Label: "Verify the TLS certificate",
			Help:  "false accepts a self-signed z/OSMF certificate. Only for test systems.",
			Kind:  KindBool, Cardinality: One, Default: "true", Group: "system",
		},
		{
			ID: "ca-file", YAMLKey: "ca_file", Label: "CA certificate file",
			Help:  "PEM file for a self-signed z/OSMF certificate — the alternative to not verifying.",
			Kind:  KindString, Cardinality: One, Group: "system",
		},
		{
			ID: "home", YAMLKey: "home", Label: "USS home directory",
			Help:  "Used by vscpdf and nvpdf, not by this server.",
			Kind:  KindString, Cardinality: One, Example: "/u/ibmuser", Group: "system",
		},
	}
}

// CatalogJSON renders a catalog for an agent that has to ask the questions.
func CatalogJSON(command string, fields []Field) ([]byte, error) {
	return json.MarshalIndent(struct {
		Command string  `json:"command"`
		Fields  []Field `json:"fields"`
	}{command, fields}, "", "  ")
}
```

- [ ] **Step 4: Write the values container**

Create `internal/setup/values.go`:

```go
package setup

// Values holds the answers, whatever collected them — flags, the wizard or
// an agent. Every value is kept as a slice so a list field and a scalar go
// through the same path.
type Values struct {
	byID map[string][]string
}

func NewValues() *Values {
	return &Values{byID: map[string][]string{}}
}

func (v *Values) Set(id string, vals []string) {
	if v.byID == nil {
		v.byID = map[string][]string{}
	}
	v.byID[id] = vals
}

func (v *Values) Has(id string) bool {
	return len(v.byID[id]) > 0
}

// One returns the single value of a scalar field, or "" when unset.
func (v *Values) One(id string) string {
	if vals := v.byID[id]; len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func (v *Values) Many(id string) []string {
	return v.byID[id]
}
```

- [ ] **Step 5: Run the tests and watch them pass**

Run: `go test -count=1 ./internal/setup/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/setup/fields.go internal/setup/values.go internal/setup/fields_test.go
git commit -m "feat(setup): field catalog shared by wizard, flags and JSON"
```

---

## Task 3: Flags from the catalog

**Files:**
- Create: `internal/setup/flags.go`
- Test: `internal/setup/flags_test.go`

**Interfaces:**
- Consumes: `Field`, `ConfigFields()`, `Values` from Task 2.
- Produces:
  - `setup.BindFlags(fs *flag.FlagSet, fields []Field) *Values`
  - `setup.Validate(fields []Field, v *Values) error`

- [ ] **Step 1: Write the failing test**

Create `internal/setup/flags_test.go`:

```go
package setup

import (
	"flag"
	"io"
	"strings"
	"testing"
)

func parse(t *testing.T, args ...string) *Values {
	t.Helper()
	fs := flag.NewFlagSet("init config", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	v := BindFlags(fs, ConfigFields())
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return v
}

func TestScalarFlag(t *testing.T) {
	v := parse(t, "--name", "MAINT")
	if got := v.One("name"); got != "MAINT" {
		t.Errorf("name = %q", got)
	}
}

// Both spellings have to work: an agent copying the documented example uses
// one, a human at a shell often the other.
func TestListFlagAcceptsRepeatAndCommas(t *testing.T) {
	repeated := parse(t, "--csi", "A.CSI", "--csi", "B.CSI").Many("csi")
	commas := parse(t, "--csi", "A.CSI,B.CSI").Many("csi")

	for _, got := range [][]string{repeated, commas} {
		if len(got) != 2 || got[0] != "A.CSI" || got[1] != "B.CSI" {
			t.Errorf("csi = %v, want [A.CSI B.CSI]", got)
		}
	}
}

func TestValidateNamesTheMissingFlag(t *testing.T) {
	err := Validate(ConfigFields(), parse(t, "--name", "MAINT"))
	if err == nil {
		t.Fatal("expected an error when host and user are missing")
	}
	for _, want := range []string{"--host", "--user"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
	// A small model correcting itself needs the example, not just the name.
	if !strings.Contains(err.Error(), "IBMUSER") {
		t.Errorf("error %q carries no example", err)
	}
}

// Flags are generated from the catalog, and this test is what keeps that
// true: a field added to one and not the other is a field an agent is told
// about but cannot pass.
func TestEveryCatalogFieldHasAFlag(t *testing.T) {
	fs := flag.NewFlagSet("init config", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	BindFlags(fs, ConfigFields())

	for _, f := range ConfigFields() {
		if fs.Lookup(f.ID) == nil {
			t.Errorf("catalog field %q has no flag", f.ID)
		}
	}
	count := 0
	fs.VisitAll(func(*flag.Flag) { count++ })
	if count != len(ConfigFields()) {
		t.Errorf("%d flags for %d fields — one side has an extra", count, len(ConfigFields()))
	}
}

func TestValidatePassesWhenRequiredArePresent(t *testing.T) {
	v := parse(t, "--name", "MAINT", "--host", "https://zos.example.com:443", "--user", "IBMUSER")
	if err := Validate(ConfigFields(), v); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test -run 'TestScalarFlag|TestListFlag|TestValidate' ./internal/setup/`
Expected: FAIL, `undefined: BindFlags`

- [ ] **Step 3: Implement**

Create `internal/setup/flags.go`:

```go
package setup

import (
	"flag"
	"fmt"
	"strings"
)

// listValue collects a repeatable flag and splits comma-separated input, so
// --csi A --csi B and --csi A,B mean the same thing.
type listValue struct{ out *[]string }

func (l listValue) String() string { return strings.Join(*l.out, ",") }

func (l listValue) Set(s string) error {
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*l.out = append(*l.out, part)
		}
	}
	return nil
}

// BindFlags registers one flag per catalog entry and returns the container
// the parsed answers land in.
func BindFlags(fs *flag.FlagSet, fields []Field) *Values {
	v := NewValues()
	for _, f := range fields {
		field := f
		usage := field.Help
		if field.Example != nil {
			usage = fmt.Sprintf("%s (e.g. %v)", usage, field.Example)
		}
		if field.Cardinality == Many {
			var out []string
			fs.Var(listValue{&out}, field.ID, usage)
			defer func() {}() // values are read after Parse, see collect below
			v.registerList(field.ID, &out)
			continue
		}
		s := fs.String(field.ID, field.Default, usage)
		v.registerScalar(field.ID, s)
	}
	return v
}

// Validate reports every missing required field at once, naming the flag and
// an example, so one correction is enough.
func Validate(fields []Field, v *Values) error {
	var problems []string
	for _, f := range fields {
		if !f.Required || v.Has(f.ID) {
			continue
		}
		msg := fmt.Sprintf("missing --%s (%s)", f.ID, strings.ToLower(strings.TrimSuffix(f.Label, ".")))
		if f.Example != nil {
			msg = fmt.Sprintf("%s, e.g. %v", msg, f.Example)
		}
		problems = append(problems, msg)
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(problems, "\n"))
}
```

- [ ] **Step 4: Add the late-binding helpers to Values**

`flag` writes into pointers during `Parse`, which happens after `BindFlags`
returns. Append to `internal/setup/values.go`:

```go
// The flag package fills its targets during Parse, after BindFlags has
// returned, so the pointers are remembered and read on access.
type binding struct {
	scalar *string
	list   *[]string
}

func (v *Values) registerScalar(id string, p *string) {
	if v.bound == nil {
		v.bound = map[string]binding{}
	}
	v.bound[id] = binding{scalar: p}
}

func (v *Values) registerList(id string, p *[]string) {
	if v.bound == nil {
		v.bound = map[string]binding{}
	}
	v.bound[id] = binding{list: p}
}

// resolve folds the flag targets into byID on first access.
func (v *Values) resolve(id string) []string {
	if vals, ok := v.byID[id]; ok {
		return vals
	}
	b, ok := v.bound[id]
	if !ok {
		return nil
	}
	switch {
	case b.list != nil && len(*b.list) > 0:
		return *b.list
	case b.scalar != nil && *b.scalar != "":
		return []string{*b.scalar}
	}
	return nil
}
```

Change the `Values` struct and the three readers to go through `resolve`:

```go
type Values struct {
	byID  map[string][]string
	bound map[string]binding
}

func (v *Values) Has(id string) bool { return len(v.resolve(id)) > 0 }

func (v *Values) One(id string) string {
	if vals := v.resolve(id); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func (v *Values) Many(id string) []string { return v.resolve(id) }
```

Remove the stray `defer func() {}()` line from `BindFlags` — it was a
placeholder and does nothing.

- [ ] **Step 5: Run the tests and watch them pass**

Run: `go test -count=1 ./internal/setup/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/setup/flags.go internal/setup/values.go internal/setup/flags_test.go
git commit -m "feat(setup): bind the catalog to flags, validate with named flags"
```

---

## Task 4: Editing the YAML without damaging it

**Files:**
- Create: `internal/setup/document.go`
- Test: `internal/setup/document_test.go`, `internal/setup/testdata/shared.yaml`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `setup.LoadDocument(path string) (*Document, error)` — an empty document when the file does not exist
  - `(*Document).UpsertSystem(name string, entries []Entry) (changed bool, err error)`
  - `(*Document).UpsertSSHSystem(name string, entries []Entry) (changed bool, err error)`
  - `(*Document).Bytes() ([]byte, error)`
  - `(*Document).Save(path string) error` — writes `<path>.bak` once, then the file
  - `setup.Entry{Key string; Value any}` where Value is `string`, `[]string`, `int` or `bool`

- [ ] **Step 1: Create the fixture**

Create `internal/setup/testdata/shared.yaml`:

```yaml
# Shared by zosmf-mcp, smpe_ls, vscode-smpe and nvpdf.
servers:
  - name: MAINT
    host: https://zos1.example.com:443
    user: IBMUSER
    home: "/u/ibmuser"          # nvpdf reads this
    csi:
      - OLD.SMPE.GLOBAL.CSI

defaultServer: MAINT
```

- [ ] **Step 2: Write the failing round-trip test**

Create `internal/setup/document_test.go`:

```go
package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("testdata/shared.yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "shared.yaml")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The file belongs to four tools. Updating our keys must not disturb theirs,
// their comments, or keys we have never heard of.
func TestUpsertKeepsForeignContent(t *testing.T) {
	path := fixture(t)
	doc, err := LoadDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.UpsertSystem("MAINT", []Entry{
		{Key: "csi", Value: []string{"NEW.SMPE.GLOBAL.CSI"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(path); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{
		"# Shared by zosmf-mcp",
		`home: "/u/ibmuser"`,
		"# nvpdf reads this",
		"defaultServer: MAINT",
		"NEW.SMPE.GLOBAL.CSI",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("result lost %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "OLD.SMPE.GLOBAL.CSI") {
		t.Errorf("old value survived:\n%s", text)
	}
}

func TestUpsertAppendsUnknownSystem(t *testing.T) {
	path := fixture(t)
	doc, _ := LoadDocument(path)
	if _, err := doc.UpsertSystem("PROD", []Entry{
		{Key: "host", Value: "https://zos2.example.com:443"},
		{Key: "user", Value: "IBMUSER"},
	}); err != nil {
		t.Fatal(err)
	}
	out, _ := doc.Bytes()
	text := string(out)
	if strings.Count(text, "name: MAINT") != 1 || !strings.Contains(text, "name: PROD") {
		t.Errorf("expected MAINT kept and PROD added:\n%s", text)
	}
}

// The server uppercases system names, so the file must not end up with two
// entries that differ only in case.
func TestUpsertMatchesNameCaseInsensitively(t *testing.T) {
	path := fixture(t)
	doc, _ := LoadDocument(path)
	if _, err := doc.UpsertSystem("maint", []Entry{{Key: "user", Value: "OTHER"}}); err != nil {
		t.Fatal(err)
	}
	out, _ := doc.Bytes()
	if n := strings.Count(string(out), "- name:"); n != 1 {
		t.Errorf("got %d systems, want 1:\n%s", n, out)
	}
}

func TestUpsertIsIdempotent(t *testing.T) {
	path := fixture(t)
	doc, _ := LoadDocument(path)
	entries := []Entry{{Key: "user", Value: "IBMUSER"}}

	first, err := doc.UpsertSystem("MAINT", entries)
	if err != nil {
		t.Fatal(err)
	}
	second, err := doc.UpsertSystem("MAINT", entries)
	if err != nil {
		t.Fatal(err)
	}
	if first {
		t.Error("writing the value it already has reported a change")
	}
	if second {
		t.Error("the second identical upsert reported a change")
	}
}

func TestLoadDocumentOfMissingFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.yaml")
	doc, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("a missing file must not be an error: %v", err)
	}
	if _, err := doc.UpsertSystem("MAINT", []Entry{{Key: "user", Value: "IBMUSER"}}); err != nil {
		t.Fatal(err)
	}
	out, _ := doc.Bytes()
	if !strings.Contains(string(out), "servers:") {
		t.Errorf("a new file must use the servers: spelling:\n%s", out)
	}
}

func TestSaveLeavesABackup(t *testing.T) {
	path := fixture(t)
	doc, _ := LoadDocument(path)
	if _, err := doc.UpsertSystem("MAINT", []Entry{{Key: "user", Value: "OTHER"}}); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("no backup next to the file: %v", err)
	}
}
```

- [ ] **Step 3: Run them and watch them fail**

Run: `go test -run 'TestUpsert|TestLoadDocument|TestSave' ./internal/setup/`
Expected: FAIL, `undefined: LoadDocument`

- [ ] **Step 4: Implement the document**

Create `internal/setup/document.go`:

```go
package setup

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Entry is one key we set on a system. Value is a string, []string, int or
// bool.
type Entry struct {
	Key   string
	Value any
}

// Document is a configuration file kept as a node tree. Editing nodes rather
// than marshalling structs is what preserves comments, key order and the
// keys other tools put in the same file.
type Document struct {
	root *yaml.Node
}

func LoadDocument(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Document{root: newMappingDoc()}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if root.Kind == 0 {
		return &Document{root: newMappingDoc()}, nil
	}
	return &Document{root: &root}, nil
}

func newMappingDoc() *yaml.Node {
	return &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{{
			Kind: yaml.MappingNode, Tag: "!!map",
		}},
	}
}

func (d *Document) UpsertSystem(name string, entries []Entry) (bool, error) {
	return d.upsert(d.systemsKey(), name, entries)
}

func (d *Document) UpsertSSHSystem(name string, entries []Entry) (bool, error) {
	return d.upsert("ssh_systems", name, entries)
}

// systemsKey keeps the spelling the file already uses and falls back to the
// one the other tools write.
func (d *Document) systemsKey() string {
	if mapValue(d.mapping(), "systems") != nil {
		return "systems"
	}
	return "servers"
}

func (d *Document) mapping() *yaml.Node {
	return d.root.Content[0]
}

func (d *Document) upsert(sectionKey, name string, entries []Entry) (bool, error) {
	section := mapValue(d.mapping(), sectionKey)
	if section == nil {
		section = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		setMapValue(d.mapping(), sectionKey, section)
	}
	if section.Kind != yaml.SequenceNode {
		return false, fmt.Errorf("%s in the configuration is not a list", sectionKey)
	}

	entry := findByName(section, name)
	changed := false
	if entry == nil {
		entry = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setMapValue(entry, "name", scalar(name))
		section.Content = append(section.Content, entry)
		changed = true
	}
	for _, e := range entries {
		node, err := valueNode(e.Value)
		if err != nil {
			return false, fmt.Errorf("%s: %w", e.Key, err)
		}
		if existing := mapValue(entry, e.Key); existing != nil && sameValue(existing, node) {
			continue
		}
		setMapValue(entry, e.Key, node)
		changed = true
	}
	return changed, nil
}

func findByName(seq *yaml.Node, name string) *yaml.Node {
	for _, item := range seq.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		if n := mapValue(item, "name"); n != nil && strings.EqualFold(n.Value, name) {
			return item
		}
	}
	return nil
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func setMapValue(m *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			// Keep the key node, and with it any comment attached to it.
			m.Content[i+1] = value
			return
		}
	}
	m.Content = append(m.Content, scalar(key), value)
}

func scalar(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

func valueNode(v any) (*yaml.Node, error) {
	switch t := v.(type) {
	case string:
		return scalar(t), nil
	case int:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprint(t)}, nil
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(t)}, nil
	case []string:
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, s := range t {
			seq.Content = append(seq.Content, scalar(s))
		}
		return seq, nil
	default:
		return nil, fmt.Errorf("unsupported value type %T", v)
	}
}

// sameValue compares rendered values, which is enough to decide whether a
// write would change anything.
func sameValue(a, b *yaml.Node) bool {
	ra, err1 := render(a)
	rb, err2 := render(b)
	return err1 == nil && err2 == nil && bytes.Equal(ra, rb)
}

func render(n *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (d *Document) Bytes() ([]byte, error) {
	return render(d.root)
}

// Save writes a backup beside the file the first time it replaces it.
func (d *Document) Save(path string) error {
	out, err := d.Bytes()
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", existing, 0o600); err != nil {
			return fmt.Errorf("writing the backup: %w", err)
		}
	}
	return os.WriteFile(path, out, 0o600)
}
```

- [ ] **Step 5: Run the tests and watch them pass**

Run: `go test -count=1 ./internal/setup/`
Expected: PASS. If the round-trip test reports a lost comment, the cause is
`setMapValue` replacing the key node instead of the value node — keep the key.

- [ ] **Step 6: Commit**

```bash
git add internal/setup/document.go internal/setup/document_test.go internal/setup/testdata/shared.yaml
git commit -m "feat(setup): edit the shared config on the node level

The file is read by smpe_ls, vscode-smpe and nvpdf as well. Marshalling
it back from structs would drop their keys and every comment."
```

---

## Task 5: `zosmf-mcp init config`

**Files:**
- Create: `cmd/zosmf-mcp/init.go`
- Modify: `cmd/zosmf-mcp/main.go` (dispatch before the server starts)
- Test: `cmd/zosmf-mcp/init_test.go`

**Interfaces:**
- Consumes: `setup.ConfigFields`, `setup.BindFlags`, `setup.Validate`, `setup.Values`, `setup.LoadDocument`, `setup.Entry`, `config.ResolveDefaultPath`, `config.Load`.
- Produces: `runInit(args []string, stdout, stderr io.Writer, isTTY bool) int` — the exit code, so tests can drive it without spawning a process.

- [ ] **Step 1: Write the failing test**

Create `cmd/zosmf-mcp/init_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, tty bool, args ...string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := runInit(args, &out, &errBuf, tty)
	return code, out.String(), errBuf.String()
}

func TestInitConfigWritesASystem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.yaml")

	code, _, errOut := run(t, false, "config", "--file", path,
		"--name", "MAINT", "--host", "https://zos.example.com:443",
		"--user", "IBMUSER", "--csi", "A.CSI,B.CSI", "--zones", "MVST100")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"name: MAINT", "A.CSI", "B.CSI", "MVST100"} {
		if !strings.Contains(text, want) {
			t.Errorf("file lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "password") {
		t.Errorf("a password reference was written:\n%s", text)
	}
}

func TestInitConfigWithoutTTYAndWithoutFlagsExplainsItself(t *testing.T) {
	code, _, errOut := run(t, false, "config", "--file", filepath.Join(t.TempDir(), "c.yaml"))
	if code == 0 {
		t.Fatal("expected a non-zero exit")
	}
	for _, want := range []string{"--questions", "--name"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr %q does not mention %q", errOut, want)
		}
	}
}

func TestInitConfigQuestionsIsJSON(t *testing.T) {
	code, out, _ := run(t, false, "config", "--questions")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var doc struct {
		Command string `json:"command"`
		Fields  []any  `json:"fields"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if doc.Command != "init config" || len(doc.Fields) == 0 {
		t.Errorf("unexpected catalog: %+v", doc)
	}
}

func TestInitConfigPrintWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.yaml")
	code, out, _ := run(t, false, "config", "--file", path, "--print",
		"--name", "MAINT", "--host", "https://zos.example.com:443", "--user", "IBMUSER")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "name: MAINT") {
		t.Errorf("stdout carries no rendered config:\n%s", out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("--print created the file")
	}
}

func TestInitConfigCheckRejectsABrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.yaml")
	if err := os.WriteFile(path, []byte("servers:\n  - name: SYS1\n    host: https://z.example.com\n    auth:\n      cert_file: /x.pem\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := run(t, false, "config", "--file", path, "--check")
	if code == 0 {
		t.Fatalf("expected a non-zero exit, stderr: %s", errOut)
	}
	if !strings.Contains(errOut, "auth type missing") {
		t.Errorf("stderr should carry the loader's complaint, got: %s", errOut)
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test -run TestInitConfig ./cmd/zosmf-mcp/`
Expected: FAIL, `undefined: runInit`

- [ ] **Step 3: Implement the command**

Create `cmd/zosmf-mcp/init.go`:

```go
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/setup"
)

// runInit handles `zosmf-mcp init …`. It returns the exit code instead of
// calling os.Exit so the tests can drive it in process.
func runInit(args []string, stdout, stderr io.Writer, isTTY bool) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: zosmf-mcp init <config|env> [flags]")
		return 2
	}
	switch args[0] {
	case "config":
		return runInitConfig(args[1:], stdout, stderr, isTTY)
	case "env":
		return runInitEnv(args[1:], stdout, stderr, isTTY)
	default:
		fmt.Fprintf(stderr, "unknown init subcommand %q — use config or env\n", args[0])
		return 2
	}
}

func runInitConfig(args []string, stdout, stderr io.Writer, isTTY bool) int {
	fs := flag.NewFlagSet("init config", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "configuration file to create or update")
	questions := fs.Bool("questions", false, "print the field catalog as JSON and exit")
	print := fs.Bool("print", false, "render the result to stdout without writing")
	check := fs.Bool("check", false, "load the file through the server's config loader and report problems")

	fields := setup.ConfigFields()
	values := setup.BindFlags(fs, fields)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *questions {
		data, err := setup.CatalogJSON("init config", fields)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, string(data))
		return 0
	}

	path, err := resolveConfigPath(*file)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if *check {
		if _, err := config.Load(path); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "%s is valid\n", path)
		return 0
	}

	anyFlag := false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "file", "questions", "print", "check":
		default:
			anyFlag = true
		}
	})

	if !anyFlag {
		if !isTTY {
			fmt.Fprintln(stderr, "no field flags given and no terminal to ask on.")
			fmt.Fprintln(stderr, "run `zosmf-mcp init config --questions` for the field list, then pass them as flags, e.g.:")
			fmt.Fprintln(stderr, "  zosmf-mcp init config --name MAINT --host https://zos.example.com:443 --user IBMUSER")
			return 2
		}
		if err := setup.AskAll(fields, values, os.Stdin, stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	if err := setup.Validate(fields, values); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	doc, err := setup.LoadDocument(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	entries := setup.EntriesFor(fields, values)
	if _, err := doc.UpsertSystem(values.One("name"), entries); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if *print {
		out, err := doc.Bytes()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprint(stdout, string(out))
		return 0
	}

	if err := doc.Save(path); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s\n", path)
	fmt.Fprintf(stdout, "next: zosmf-mcp init env --file %s\n", path)
	return 0
}

// resolveConfigPath honours --file, then the server's own search order, and
// falls back to the shared location when nothing exists yet.
func resolveConfigPath(fileFlag string) (string, error) {
	if fileFlag != "" {
		return fileFlag, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	home, _ := os.UserHomeDir()
	path, _, err := config.ResolveDefaultPath(cwd, home)
	if err == nil {
		return path, nil
	}
	if home == "" {
		return "", fmt.Errorf("no configuration found and no home directory to put one in — pass --file")
	}
	return home + "/.config/smpe_ls/" + config.DefaultConfigName, nil
}
```

- [ ] **Step 4: Add the catalog-to-entries helper**

Append to `internal/setup/fields.go`:

```go
// EntriesFor turns the answers into the keys to write, skipping the ones
// that were not answered and the name, which identifies the entry.
func EntriesFor(fields []Field, v *Values) []Entry {
	var entries []Entry
	for _, f := range fields {
		if f.ID == "name" || !v.Has(f.ID) {
			continue
		}
		switch {
		case f.Cardinality == Many:
			entries = append(entries, Entry{Key: f.YAMLKey, Value: v.Many(f.ID)})
		case f.Kind == KindBool:
			entries = append(entries, Entry{Key: f.YAMLKey, Value: v.One(f.ID) == "true"})
		case f.Kind == KindInt:
			var n int
			fmt.Sscanf(v.One(f.ID), "%d", &n)
			entries = append(entries, Entry{Key: f.YAMLKey, Value: n})
		default:
			entries = append(entries, Entry{Key: f.YAMLKey, Value: v.One(f.ID)})
		}
	}
	return entries
}
```

Add `"fmt"` to the imports of `fields.go`.

- [ ] **Step 5: Add a placeholder wizard so the package compiles**

Create `internal/setup/prompt.go` with the signature Task 6 fills in:

```go
package setup

import (
	"errors"
	"io"
)

// AskAll walks the catalog and asks for each field. Task 6 implements it.
func AskAll(fields []Field, v *Values, in io.Reader, out io.Writer) error {
	return errors.New("the interactive wizard is not implemented yet — pass the fields as flags")
}
```

- [ ] **Step 6: Dispatch from main**

In `cmd/zosmf-mcp/main.go`, as the first statement in `main()`, before
`flag.String` is called:

```go
	// `init` is a separate command with its own flags; it must not reach the
	// server's flag set.
	if len(os.Args) > 1 && os.Args[1] == "init" {
		fi, _ := os.Stdin.Stat()
		isTTY := fi != nil && fi.Mode()&os.ModeCharDevice != 0
		os.Exit(runInit(os.Args[2:], os.Stdout, os.Stderr, isTTY))
	}
```

- [ ] **Step 7: Run the tests and watch them pass**

Run: `go test -count=1 ./cmd/zosmf-mcp/ ./internal/setup/`
Expected: PASS

- [ ] **Step 8: Try it by hand**

Run:
```bash
make build
./bin/zosmf-mcp init config --questions | head -20
./bin/zosmf-mcp init config --file /tmp/probe.yaml --name MAINT \
  --host https://zos.example.com:443 --user IBMUSER --csi A.CSI,B.CSI --print
```
Expected: a JSON catalog, then a rendered configuration with both CSI
datasets and no password key.

- [ ] **Step 9: Commit**

```bash
git add cmd/zosmf-mcp/init.go cmd/zosmf-mcp/init_test.go cmd/zosmf-mcp/main.go internal/setup/fields.go internal/setup/prompt.go
git commit -m "feat(cli): zosmf-mcp init config"
```

---

## Task 6: The interactive wizard

**Files:**
- Modify: `internal/setup/prompt.go` (replace the placeholder from Task 5)
- Test: `internal/setup/prompt_test.go`

**Interfaces:**
- Consumes: `Field`, `Values` from Task 2.
- Produces: `setup.AskAll(fields []Field, v *Values, in io.Reader, out io.Writer) error` — unchanged signature, now implemented.

- [ ] **Step 1: Write the failing test**

Create `internal/setup/prompt_test.go`:

```go
package setup

import (
	"bytes"
	"strings"
	"testing"
)

func TestAskAllCollectsAnswers(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		"MAINT",                        // name
		"https://zos.example.com:443",  // host
		"",                             // port, skipped
		"IBMUSER",                      // user
		"A.CSI, B.CSI",                 // csi
		"MVST100",                      // zones
		"",                             // default-zones
		"", "", "", "", "",             // the remaining optional fields
	}, "\n") + "\n")

	v := NewValues()
	var out bytes.Buffer
	if err := AskAll(ConfigFields(), v, in, &out); err != nil {
		t.Fatalf("AskAll: %v", err)
	}

	if v.One("name") != "MAINT" {
		t.Errorf("name = %q", v.One("name"))
	}
	if got := v.Many("csi"); len(got) != 2 || got[1] != "B.CSI" {
		t.Errorf("csi = %v, want two entries", got)
	}
	if v.Has("port") {
		t.Error("an empty answer must not become a value")
	}
}

// A required field that is left empty is asked again rather than accepted.
func TestAskAllRepeatsRequiredFields(t *testing.T) {
	in := strings.NewReader("\nMAINT\nhttps://zos.example.com:443\n\nIBMUSER\n\n\n\n\n\n\n\n")
	v := NewValues()
	var out bytes.Buffer
	if err := AskAll(ConfigFields(), v, in, &out); err != nil {
		t.Fatalf("AskAll: %v", err)
	}
	if v.One("name") != "MAINT" {
		t.Errorf("name = %q, want the answer after the empty one", v.One("name"))
	}
	if !strings.Contains(out.String(), "required") {
		t.Errorf("the user was not told the field is required:\n%s", out.String())
	}
}

// The consequence of skipping csi and zones is spelled out, because the
// failure otherwise appears much later as a tool error.
func TestAskAllWarnsWhenSMPEFieldsAreSkipped(t *testing.T) {
	in := strings.NewReader("MAINT\nhttps://zos.example.com:443\n\nIBMUSER\n\n\n\n\n\n\n\n\n")
	v := NewValues()
	var out bytes.Buffer
	if err := AskAll(ConfigFields(), v, in, &out); err != nil {
		t.Fatalf("AskAll: %v", err)
	}
	if !strings.Contains(out.String(), "SMP/E") {
		t.Errorf("no warning about the SMP/E tools:\n%s", out.String())
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test -run TestAskAll ./internal/setup/`
Expected: FAIL — the placeholder returns an error.

- [ ] **Step 3: Implement the wizard**

Replace `internal/setup/prompt.go`:

```go
package setup

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// AskAll walks the catalog once, asking for every field. An empty answer
// leaves an optional field unset; a required field is asked again.
func AskAll(fields []Field, v *Values, in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)
	for _, f := range fields {
		for {
			prompt(out, f)
			line, err := r.ReadString('\n')
			if err != nil && line == "" {
				if f.Required {
					return fmt.Errorf("input ended before %s was answered", f.ID)
				}
				break
			}
			answer := strings.TrimSpace(line)
			if answer == "" {
				if f.Required {
					fmt.Fprintf(out, "  %s is required.\n", f.ID)
					continue
				}
				if f.Default != "" {
					v.Set(f.ID, []string{f.Default})
				}
				break
			}
			if f.Cardinality == Many {
				var vals []string
				for _, part := range strings.Split(answer, ",") {
					if part = strings.TrimSpace(part); part != "" {
						vals = append(vals, part)
					}
				}
				v.Set(f.ID, vals)
			} else {
				v.Set(f.ID, []string{answer})
			}
			break
		}
	}

	if !v.Has("csi") || !v.Has("zones") {
		fmt.Fprintln(out, "\nNote: without csi and zones every SMP/E tool call has to name them,")
		fmt.Fprintln(out, "or it fails with \"no csi_dataset provided\" / \"no zone provided\".")
	}
	fmt.Fprintln(out, "\nThe password is not stored in this file. Run `zosmf-mcp init env` next.")
	return nil
}

func prompt(out io.Writer, f Field) {
	marker := ""
	if f.Required {
		marker = " (required)"
	}
	fmt.Fprintf(out, "\n%s%s\n  %s\n", f.Label, marker, f.Help)
	if f.Cardinality == Many {
		fmt.Fprintln(out, "  several values, comma separated")
	}
	if f.Example != nil {
		fmt.Fprintf(out, "  example: %v\n", f.Example)
	}
	if f.Default != "" {
		fmt.Fprintf(out, "  default: %s\n", f.Default)
	}
	fmt.Fprint(out, "> ")
}
```

- [ ] **Step 4: Run them and watch them pass**

Run: `go test -count=1 ./internal/setup/`
Expected: PASS

- [ ] **Step 5: Try it by hand**

Run: `make build && ./bin/zosmf-mcp init config --file /tmp/wizard.yaml`
Expected: the questions appear one by one; an empty system name is refused;
the closing note points at `init env`.

- [ ] **Step 6: Commit**

```bash
git add internal/setup/prompt.go internal/setup/prompt_test.go
git commit -m "feat(setup): interactive wizard driven by the field catalog"
```

---

## Task 7: `zosmf-mcp init env`

**Files:**
- Create: `internal/setup/env.go`
- Modify: `cmd/zosmf-mcp/init.go` (`runInitEnv`)
- Test: `internal/setup/env_test.go`, `cmd/zosmf-mcp/init_env_test.go`

**Interfaces:**
- Consumes: `config.Load`, `config.PasswordEnvVar` (Task 1), `Values` (Task 2).
- Produces:
  - `setup.RequiredEnvVars(cfg *config.Config, raw []byte) []string` — the password variables the configuration needs, in file order, plus `MCP_API_KEY` when an HTTP transport is enabled
  - `setup.WriteEnvFile(path string, vars map[string]string) error` — merges into an existing file, mode 600, no `export`
  - `setup.GenerateAPIKey() (string, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/setup/env_test.go`:

```go
package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteEnvFileMergesAndKeepsMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("EXISTING=keep\nZOSMF_MAINT_PASSWORD=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteEnvFile(path, map[string]string{"ZOSMF_MAINT_PASSWORD": "new"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "EXISTING=keep") {
		t.Errorf("an unrelated variable was dropped:\n%s", text)
	}
	if strings.Contains(text, "=old") || !strings.Contains(text, "ZOSMF_MAINT_PASSWORD=new") {
		t.Errorf("the value was not replaced:\n%s", text)
	}
	if strings.Contains(text, "export ") {
		t.Errorf("export form is rejected by --env-file:\n%s", text)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 600", info.Mode().Perm())
	}
}

func TestGenerateAPIKeyIsLongAndUnique(t *testing.T) {
	a, err := GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := GenerateAPIKey()
	if len(a) < 32 {
		t.Errorf("key is %d characters, want at least 32", len(a))
	}
	if a == b {
		t.Error("two calls produced the same key")
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test -run 'TestWriteEnvFile|TestGenerateAPIKey' ./internal/setup/`
Expected: FAIL, `undefined: WriteEnvFile`

- [ ] **Step 3: Implement the env file handling**

Create `internal/setup/env.go`:

```go
package setup

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
)

// RequiredEnvVars lists the variables this configuration needs: one password
// per system, by convention, plus the API key when an HTTP transport is on.
// Variables the file already references explicitly are included as well.
func RequiredEnvVars(cfg *config.Config, raw []byte) []string {
	seen := map[string]bool{}
	var vars []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			vars = append(vars, name)
		}
	}
	for _, sys := range cfg.Systems {
		add(config.PasswordEnvVar(sys.Name))
	}
	for _, name := range referencedVars(string(raw)) {
		add(name)
	}
	if cfg.Transport.HTTP.Enabled || cfg.Transport.StreamableHTTP.Enabled {
		add("MCP_API_KEY")
	}
	return vars
}

// referencedVars finds ${NAME} references so a hand-written file keeps
// working alongside the convention.
func referencedVars(text string) []string {
	var out []string
	for {
		start := strings.Index(text, "${")
		if start < 0 {
			return out
		}
		end := strings.Index(text[start:], "}")
		if end < 0 {
			return out
		}
		out = append(out, text[start+2:start+end])
		text = text[start+end:]
	}
}

// WriteEnvFile merges the given variables into an existing file, keeping
// everything else, and writes it back with mode 600 in plain KEY=value form.
func WriteEnvFile(path string, vars map[string]string) error {
	existing := map[string]string{}
	var order []string

	if f, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			line = strings.TrimPrefix(line, "export ")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, found := strings.Cut(line, "=")
			if !found {
				continue
			}
			if _, known := existing[key]; !known {
				order = append(order, key)
			}
			existing[key] = value
		}
		_ = f.Close()
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	var added []string
	for key := range vars {
		if _, known := existing[key]; !known {
			added = append(added, key)
		}
		existing[key] = vars[key]
	}
	sort.Strings(added)
	order = append(order, added...)

	var b strings.Builder
	for _, key := range order {
		fmt.Fprintf(&b, "%s=%s\n", key, existing[key])
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// GenerateAPIKey produces a bearer token of the same shape as `make apikey`.
func GenerateAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating an API key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}
```

- [ ] **Step 4: Run them and watch them pass**

Run: `go test -count=1 ./internal/setup/`
Expected: PASS

- [ ] **Step 5: Write the failing command test**

Create `cmd/zosmf-mcp/init_env_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitEnvQuestionsListsTheSystemVariables(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "conf.yaml")
	if err := os.WriteFile(conf, []byte(`
servers:
  - name: MAINT
    host: https://zos.example.com:443
    user: IBMUSER
  - name: PROD
    host: https://zos2.example.com:443
    user: IBMUSER
`), 0o600); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := run(t, false, "env", "--file", conf, "--questions")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	for _, want := range []string{"ZOSMF_MAINT_PASSWORD", "ZOSMF_PROD_PASSWORD"} {
		if !strings.Contains(out, want) {
			t.Errorf("catalog lacks %q:\n%s", want, out)
		}
	}
}

func TestInitEnvWritesAKeyWithoutPrintingIt(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "conf.yaml")
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(conf, []byte(`
servers:
  - name: MAINT
    host: https://zos.example.com:443
    user: IBMUSER
`), 0o600); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := run(t, false, "env", "--file", conf, "--env-file", envFile, "--generate-api-key")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}

	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "MCP_API_KEY=") {
		t.Errorf(".env has no API key:\n%s", data)
	}
	key := strings.TrimPrefix(strings.TrimSpace(string(data)), "MCP_API_KEY=")
	if key != "" && strings.Contains(out, key) {
		t.Error("the generated key was printed to stdout")
	}
}
```

- [ ] **Step 6: Run them and watch them fail**

Run: `go test -run TestInitEnv ./cmd/zosmf-mcp/`
Expected: FAIL — `runInitEnv` is still the stub from Task 5.

- [ ] **Step 7: Implement `runInitEnv`**

In `cmd/zosmf-mcp/init.go`, add:

```go
func runInitEnv(args []string, stdout, stderr io.Writer, isTTY bool) int {
	fs := flag.NewFlagSet("init env", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "configuration file to read the required variables from")
	envFile := fs.String("env-file", ".env", "environment file to write")
	questions := fs.Bool("questions", false, "print the required variables as JSON and exit")
	genKey := fs.Bool("generate-api-key", false, "generate MCP_API_KEY instead of asking for it")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	path, err := resolveConfigPath(*file)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(stderr, "%v\nfix the configuration first: zosmf-mcp init config --check --file %s\n", err, path)
		return 1
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	needed := setup.RequiredEnvVars(cfg, raw)

	if *questions {
		fields := make([]setup.Field, 0, len(needed))
		for _, name := range needed {
			fields = append(fields, setup.Field{
				ID: name, YAMLKey: "", Label: name, Group: "env",
				Kind: setup.KindString, Cardinality: setup.One, Required: true,
				Help: "Secret. Written to the environment file, never to the configuration.",
			})
		}
		data, err := setup.CatalogJSON("init env", fields)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, string(data))
		return 0
	}

	vars := map[string]string{}
	for _, name := range needed {
		if name == "MCP_API_KEY" && *genKey {
			key, err := setup.GenerateAPIKey()
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			vars[name] = key
			continue
		}
		if value := os.Getenv(name); value != "" {
			vars[name] = value
			continue
		}
		if !isTTY {
			fmt.Fprintf(stderr, "%s is not set and there is no terminal to ask on.\n", name)
			fmt.Fprintf(stderr, "export it and run again, or use --generate-api-key for MCP_API_KEY.\n")
			return 2
		}
		value, err := setup.AskSecret(name, os.Stdin, stdout)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		vars[name] = value
	}

	if err := setup.WriteEnvFile(*envFile, vars); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s with %d variable(s), mode 600\n", *envFile, len(vars))
	fmt.Fprintln(stdout, "values are not printed here — read them from the file if you need them")
	return 0
}
```

- [ ] **Step 8: Add the secret prompt**

Append to `internal/setup/prompt.go`:

```go
// AskSecret reads one secret. The value is never echoed back by us; a
// terminal that echoes input is the caller's environment, not our doing.
func AskSecret(name string, in io.Reader, out io.Writer) (string, error) {
	fmt.Fprintf(out, "\n%s\n  Secret. It goes into the environment file, never into the configuration.\n> ", name)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("input ended before %s was answered", name)
	}
	return strings.TrimSpace(line), nil
}
```

- [ ] **Step 9: Run everything and watch it pass**

Run: `env -u MCP_API_KEY go test -count=1 ./cmd/zosmf-mcp/ ./internal/setup/ ./internal/config/`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add internal/setup/env.go internal/setup/env_test.go internal/setup/prompt.go cmd/zosmf-mcp/init.go cmd/zosmf-mcp/init_env_test.go
git commit -m "feat(cli): zosmf-mcp init env"
```

---

## Task 8: ZOAU systems

**Files:**
- Modify: `internal/setup/fields.go` (add `SSHFields()`), `cmd/zosmf-mcp/init.go` (`--with-zoau`)
- Test: `internal/setup/ssh_test.go`

**Interfaces:**
- Consumes: `Field`, `Values`, `Document.UpsertSSHSystem` (Task 4).
- Produces:
  - `setup.SSHFields() []Field` — group `zoau`
  - `setup.ZOAUEnvironment(installPath string) []Entry` — the three derived variables

- [ ] **Step 1: Write the failing test**

Create `internal/setup/ssh_test.go`:

```go
package setup

import "testing"

// One answer, three variables: nobody should have to know that LIBPATH
// points at lib.
func TestZOAUEnvironmentDerivesThreeVariables(t *testing.T) {
	entries := ZOAUEnvironment("/var/devops/zoautil")
	if len(entries) != 1 || entries[0].Key != "environment" {
		t.Fatalf("expected one environment entry, got %+v", entries)
	}
	env, ok := entries[0].Value.(map[string]string)
	if !ok {
		t.Fatalf("value is %T, want map[string]string", entries[0].Value)
	}
	for key, want := range map[string]string{
		"ZOAU_HOME": "/var/devops/zoautil",
		"PATH":      "/var/devops/zoautil/bin:/bin:/usr/bin",
		"LIBPATH":   "/var/devops/zoautil/lib:/lib:/usr/lib",
	} {
		if env[key] != want {
			t.Errorf("%s = %q, want %q", key, env[key], want)
		}
	}
}

func TestSSHFieldsCoverWhatRunSSHNeeds(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range SSHFields() {
		seen[f.ID] = true
		if f.Group != "zoau" {
			t.Errorf("field %s is in group %q, want zoau", f.ID, f.Group)
		}
	}
	for _, want := range []string{"ssh-host", "ssh-user", "ssh-key-file", "zoau-path"} {
		if !seen[want] {
			t.Errorf("SSH catalog is missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run them and watch them fail**

Run: `go test -run 'TestZOAU|TestSSHFields' ./internal/setup/`
Expected: FAIL, `undefined: ZOAUEnvironment`

- [ ] **Step 3: Implement**

Create `internal/setup/ssh.go`:

```go
package setup

import "strings"

// SSHFields is the catalog for the ZOAU tools, which reach z/OS over SSH
// rather than through z/OSMF.
func SSHFields() []Field {
	return []Field{
		{
			ID: "ssh-host", YAMLKey: "host", Label: "SSH host",
			Help:  "The LPAR's SSH address. Usually the host part of the z/OSMF URL.",
			Kind:  KindString, Cardinality: One, Required: true,
			Example: "zos.example.com", Group: "zoau",
		},
		{
			ID: "ssh-port", YAMLKey: "port", Label: "SSH port",
			Help: "Defaults to 22.", Kind: KindInt, Cardinality: One,
			Default: "22", Group: "zoau",
		},
		{
			ID: "ssh-user", YAMLKey: "username", Label: "SSH user",
			Help:  "The z/OS user the ZOAU commands run as.",
			Kind:  KindString, Cardinality: One, Required: true,
			Example: "ibmuser", Group: "zoau",
		},
		{
			ID: "ssh-key-file", YAMLKey: "key_file", Label: "SSH private key",
			Help:  "Only key authentication is supported. In a container this is the path inside it, e.g. /keys/id_rsa.",
			Kind:  KindString, Cardinality: One, Required: true,
			Example: "~/.ssh/id_rsa", Group: "zoau",
		},
		{
			ID: "zoau-path", YAMLKey: "", Label: "ZOAU installation path",
			Help:  "ZOAU_HOME, PATH and LIBPATH are derived from it.",
			Kind:  KindString, Cardinality: One, Required: true,
			Example: "/usr/lpp/IBM/zoautil", Group: "zoau",
		},
	}
}

// ZOAUEnvironment turns the installation path into the three variables
// RunSSH exports before every command.
func ZOAUEnvironment(installPath string) []Entry {
	p := strings.TrimRight(installPath, "/")
	return []Entry{{Key: "environment", Value: map[string]string{
		"ZOAU_HOME": p,
		"PATH":      p + "/bin:/bin:/usr/bin",
		"LIBPATH":   p + "/lib:/lib:/usr/lib",
	}}}
}
```

- [ ] **Step 4: Teach the document about string maps**

In `internal/setup/document.go`, in `valueNode`, add before `default:`:

```go
	case map[string]string:
		m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			m.Content = append(m.Content, scalar(k), scalar(t[k]))
		}
		return m, nil
```

Add `"sort"` to the imports of `document.go`.

- [ ] **Step 5: Run them and watch them pass**

Run: `go test -count=1 ./internal/setup/`
Expected: PASS

- [ ] **Step 6: Wire the flag into the command**

In `runInitConfig`, after the system upsert and before `--print`:

```go
	if values.Has("zoau-path") {
		sshEntries := setup.EntriesFor(setup.SSHFields(), values)
		sshEntries = append(sshEntries, setup.ZOAUEnvironment(values.One("zoau-path"))...)
		if _, err := doc.UpsertSSHSystem(values.One("name"), sshEntries); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
```

and register the SSH fields alongside the system fields:

```go
	fields := setup.ConfigFields()
	sshFields := setup.SSHFields()
	values := setup.BindFlags(fs, append(append([]setup.Field{}, fields...), sshFields...))
```

`EntriesFor` skips fields with an empty `YAMLKey`, so `zoau-path` does not
land in the file by itself. Add that guard:

```go
	for _, f := range fields {
		if f.ID == "name" || f.YAMLKey == "" || !v.Has(f.ID) {
			continue
		}
```

- [ ] **Step 7: Write the failing command test**

Append to `cmd/zosmf-mcp/init_test.go`:

```go
func TestInitConfigWritesSSHSystemWhenZOAUPathGiven(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.yaml")
	code, _, errOut := run(t, false, "config", "--file", path,
		"--name", "MAINT", "--host", "https://zos.example.com:443", "--user", "IBMUSER",
		"--ssh-host", "zos.example.com", "--ssh-user", "ibmuser",
		"--ssh-key-file", "/keys/id_rsa", "--zoau-path", "/usr/lpp/IBM/zoautil")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	for _, want := range []string{"ssh_systems:", "key_file: /keys/id_rsa", "ZOAU_HOME:", "LIBPATH:"} {
		if !strings.Contains(text, want) {
			t.Errorf("file lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "zoau-path:") {
		t.Errorf("the helper answer leaked into the file:\n%s", text)
	}
}
```

- [ ] **Step 8: Run everything and watch it pass**

Run: `env -u MCP_API_KEY go test -count=1 ./cmd/zosmf-mcp/ ./internal/setup/`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add internal/setup/ssh.go internal/setup/ssh_test.go internal/setup/document.go cmd/zosmf-mcp/init.go cmd/zosmf-mcp/init_test.go
git commit -m "feat(setup): configure ZOAU systems from one installation path"
```

---

## Task 9: Find out how opencode and pi load instructions

**Files:**
- Modify: `docs/2026-09-21-guided-setup-design.md` (replace section 8 with the answer)

**Interfaces:**
- Consumes: nothing.
- Produces: the two file paths and formats Task 10 needs.

This task produces knowledge, not code. It exists because guessing here
would produce wrappers that the agents silently never load.

- [ ] **Step 1: Look it up**

For each of opencode and pi, find from their own documentation:
- the directory an instruction or skill file goes in, for project scope and
  for user scope
- the file name and whether front matter is required, and in what format
- how the user invokes it

Prefer the projects' own docs over blog posts. Record the source URL.

- [ ] **Step 2: Verify by running, not by reading**

Place a minimal file at the documented location that prints one distinctive
sentence, then start the agent and invoke it. If the sentence does not
appear, the location or the format is wrong — try again before writing it
down.

- [ ] **Step 3: Record the answer in the design document**

Replace section 8 of `docs/2026-09-21-guided-setup-design.md` with a short
table: agent, project path, user path, format, invocation, source URL.

- [ ] **Step 4: Commit**

```bash
git add docs/2026-09-21-guided-setup-design.md
git commit -m "docs: record where opencode and pi look for instructions"
```

---

## Task 10: The agent skill and its installer

**Files:**
- Create: `agents/setup.md`, `agents/claude/SKILL.md`, and the two wrappers from Task 9
- Modify: `Makefile` (`install-skill`), `.PHONY` line
- Test: manual, described below

**Interfaces:**
- Consumes: the commands from Tasks 5, 7 and 8; the paths from Task 9.
- Produces: `make install-skill [AGENT=claude|opencode|pi] [SCOPE=project|user]`

- [ ] **Step 1: Write the canonical procedure**

Create `agents/setup.md`. It must contain, in this order: the seven steps
from section 5 of the spec as a numbered list, each with the exact command
and the check that follows it; the three prohibitions (no hand-written YAML,
no printing secrets, show errors verbatim and retry); and the client
registration table. Keep it under 120 lines — the detail lives in
`--questions`, not here.

The commands to name, verbatim:

```bash
zosmf-mcp --version
zosmf-mcp init config --questions
zosmf-mcp init config --name <NAME> --host <URL> --user <USERID> [--csi A,B] [--zones A,B]
zosmf-mcp init config --check
zosmf-mcp init env --questions
zosmf-mcp init env --generate-api-key
```

- [ ] **Step 2: Write the Claude wrapper**

Create `agents/claude/SKILL.md`:

```markdown
---
name: zosmf-mcp-setup
description: Set up the zosmf-mcp server — configuration file, credentials, client registration, optional container. Use when a user wants to install, configure or connect zosmf-mcp.
---

Follow `setup.md` in this directory, step by step. Do not skip the checks:
each step has one, and a failed check means stop and fix, not continue.
```

- [ ] **Step 3: Add the installer**

In the `Makefile`, after the `launch-agent-remove` target:

```make
# Copies the setup instructions into an agent's own directory. Nothing is
# installed anywhere without this being run on purpose.
AGENT ?= claude
SCOPE ?= project

install-skill:
	@./scripts/install-skill.sh $(AGENT) $(SCOPE)
```

Add `install-skill` to `.PHONY`.

- [ ] **Step 4: Write the installer script**

Create `scripts/install-skill.sh` and `chmod +x` it. The two paths marked
`FROM TASK 9` are filled in with what that task recorded:

```sh
#!/bin/sh
#
# Copies the setup instructions into an agent's own directory.
#
#   scripts/install-skill.sh [claude|opencode|pi] [project|user]

set -eu

AGENT=${1:-claude}
SCOPE=${2:-project}
REPO=$(cd "$(dirname "$0")/.." && pwd)

case "$AGENT" in
claude)
	WRAPPER="$REPO/agents/claude/SKILL.md"
	case "$SCOPE" in
	project) TARGET="$REPO/.claude/skills/zosmf-mcp-setup" ;;
	user)    TARGET="$HOME/.claude/skills/zosmf-mcp-setup" ;;
	*) echo "unknown scope $SCOPE — use project or user" >&2; exit 2 ;;
	esac
	;;
opencode)
	WRAPPER="$REPO/agents/opencode/<file from task 9>"
	TARGET="<path from task 9>"          # FROM TASK 9
	;;
pi)
	WRAPPER="$REPO/agents/pi/<file from task 9>"
	TARGET="<path from task 9>"          # FROM TASK 9
	;;
*)
	echo "unknown agent $AGENT — known: claude, opencode, pi" >&2
	exit 2
	;;
esac

mkdir -p "$TARGET"
cp "$WRAPPER" "$TARGET/"
cp "$REPO/agents/setup.md" "$TARGET/"

echo "installed the zosmf-mcp setup instructions for $AGENT ($SCOPE):"
echo "  $TARGET"
```

The wrapper and `setup.md` are copied together on purpose: the wrapper
refers to the file next to it, and a copy of only one of the two leaves an
agent pointing at nothing.

- [ ] **Step 5: Verify the Claude path by running it**

Run:
```bash
make install-skill AGENT=claude SCOPE=project
ls .claude/skills/zosmf-mcp-setup/
```
Expected: `SKILL.md` and `setup.md`. Then start Claude Code in this
repository and confirm the skill is listed.

- [ ] **Step 6: Run the whole setup against a throwaway file**

In a scratch directory, with `PATH` pointing at the built binary, let an
agent follow `agents/setup.md` and produce a configuration for a fictional
system (`zos.example.com`, user `IBMUSER`). Compare the result against a
hand-written file. They must match apart from key order.

Repeat with a local model through opencode. Note every place the model went
wrong; each one is either a missing check in `setup.md` or a vague error
message in the binary, and both are fixable here.

- [ ] **Step 7: Commit**

```bash
git add agents scripts/install-skill.sh Makefile
git commit -m "feat(agents): portable setup instructions and an installer"
```

---

## Task 11: Documentation and release

**Files:**
- Modify: `README.md`, `CHANGELOG.md`, `docs/container-setup.md`

**Interfaces:**
- Consumes: everything above.
- Produces: the released state for v0.7.0.

- [ ] **Step 1: Add "Getting started" to the README**

Directly after the "Before You Start" warning block, a section of at most 25
lines: install the binary, `make install-skill`, and the two commands with an
example. It ends with a pointer to the manual configuration section, which
stays where it is — the wizard automates it, it does not replace it.

- [ ] **Step 2: Note the password convention in the configuration section**

In the field table's vicinity, one paragraph: a password can be named
explicitly with `${...}`, otherwise the server looks for
`ZOSMF_<SYSTEM>_PASSWORD` and then `ZOSMF_PASSWORD`, and the startup log says
which one was used.

- [ ] **Step 3: Cross-reference the container guide**

In `docs/container-setup.md` section 9, one line: the setup skill covers this
path as its step 6.

- [ ] **Step 4: Write the changelog entry**

Under `## [Unreleased]`, replacing "Nothing yet.":

```markdown
### Added

- **Guided setup.** `zosmf-mcp init config` and `zosmf-mcp init env` create
  the configuration and the environment file — as an interactive wizard, or
  through flags for agents. `--questions` prints the field catalog as JSON,
  `--print` renders without writing, `--check` validates through the server's
  own loader.
- **Portable setup instructions** under `agents/`, installed with
  `make install-skill`, usable by Claude Code, opencode and pi.
- A system's password is found under `ZOSMF_<SYSTEM>_PASSWORD` without the
  configuration naming it; an explicit `${...}` still wins, `ZOSMF_PASSWORD`
  remains the fallback. The startup log names the source per system.
```

- [ ] **Step 5: Verify everything**

Run: `make fmt && make vet && env -u MCP_API_KEY make test && make lint && make check-identifiers && make build-all`
Expected: all green.

- [ ] **Step 6: Commit and open the pull request**

```bash
git add README.md CHANGELOG.md docs/container-setup.md
git commit -m "docs: getting started, password convention, release notes"
git push -u origin feat/setup-wizard
```

Then release as described in `CONTRIBUTING.md`: rename `## [Unreleased]` to
`## [0.7.0] — <date>`, tag `v0.7.0`, push the tag.

---

## Notes for the executor

- Tasks 1 through 8 are strictly ordered; each one builds on the interfaces
  of the previous.
- Task 9 can be done at any point before Task 10 and needs no code.
- If a test in Task 4 fails on a detail of how `yaml.v3` renders, fix the
  test's expectation only when the rendered output is genuinely equivalent —
  a lost comment or a dropped key is never equivalent.
- The check `git diff --stat` after each commit should show only the files
  the task names. Anything else means scope crept in.
