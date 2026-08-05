# Tool Descriptions and Annotations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every MCP tool self-describing — the model must be able to pick the right tool, fill the right system name, and know the blast radius without a trial call.

**Architecture:** Three mechanical changes plus a prose pass. (1) `mcp.Tool` gains an `Annotations` field carrying the MCP standard hints, so clients can auto-approve reads and gate writes. (2) The `system` property of every input schema gains an `enum` filled at `Definition()` time from the already-injected registry, so the model sees the configured system names instead of guessing. (3) The ZOAU tools move to their own `zoau_*` namespace with an `ssh_system` parameter, ending the collision with the z/OSMF namespace. (4) Thin descriptions are brought up to the standard already set by `zosmf_list_members`: what it does, what comes back, what must be configured first.

**Tech Stack:** Go 1.26.1, stdlib only. No new dependencies. Tests with `go test -race`.

## Global Constraints

- No new Go dependencies. The module depends only on `github.com/google/uuid` and `gopkg.in/yaml.v3` — it stays that way.
- All builds and tests run through `make` (`make build`, `make test`, `make vet`, `make fmt`).
- Never add a `Co-Authored-By` trailer to a commit message.
- `Definition()` must stay allocation-cheap but has no hot-path requirement — it is called on `tools/list`.
- Every `Definition()` in the repo keeps its existing `mustJSON(map[string]any{...})` schema style. Do not introduce a schema builder DSL.
- The ZOAU rename is a **hard breaking change with no alias fallback** — this was decided explicitly. Old tool names must not remain registered.
- Existing behaviour of `Execute()` must not change in this plan. Only `Definition()` bodies, the two registry helpers, the protocol type, and docs are touched.

---

## File Structure

**Modified:**

- `internal/mcp/protocol.go` — add `ToolAnnotations`, add `Tool.Annotations`
- `internal/zosmf/client.go:53` — make `ClientRegistry.SystemNames()` deterministic
- `internal/zosmf/ssh.go` — add `SSHClientRegistry.SystemNames()`
- `internal/tools/annotations.go` *(new)* — the two annotation constructors and the `system` schema helper, shared by all tool files
- `internal/tools/{jobs,auth,datasets,filesystems,ussfiles,console,tso,smpe,parmlib}.go` — `Definition()` bodies only
- `internal/tools/zoau.go` — `Definition()` bodies plus rename
- `internal/tools/registry.go` — constructor renames for ZOAU
- `README.md`, `.zosmf-mcp-config.example.yaml` — document the rename

**Created:**

- `internal/mcp/annotations_test.go`
- `internal/tools/annotations_test.go`
- `internal/tools/definitions_test.go` — grows across tasks 3–8; task 8 adds the completeness gate

Rationale for `annotations.go` as a separate file: the helpers are consumed by all ten tool files. Putting them in `jobs.go` next to `mustJSON` would make `jobs.go` the de-facto shared header, which it already half is. One focused file is easier to review than a growing grab bag.

---

### Task 1: Tool annotations in the protocol layer

**Files:**
- Modify: `internal/mcp/protocol.go:155-159`
- Test: `internal/mcp/annotations_test.go` (create)

**Interfaces:**
- Consumes: nothing
- Produces: `mcp.ToolAnnotations` struct with fields `Title string`, `ReadOnlyHint bool`, `DestructiveHint bool`, `IdempotentHint bool`, `OpenWorldHint bool`; and `mcp.Tool.Annotations *ToolAnnotations`. Every later task depends on these exact names.

**Design note for the implementer:** the hint fields deliberately have **no** `omitempty`. The MCP spec defines the default of `destructiveHint` as `true` — if we omitted a `false` value, a client would read "not destructive" as "destructive". Serialising all four explicitly removes that trap. `Annotations` itself is a pointer with `omitempty` so a tool that has none emits no key at all.

- [ ] **Step 1: Write the failing test**

Create `internal/mcp/annotations_test.go`:

```go
package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTool_OmitsAnnotationsWhenAbsent(t *testing.T) {
	data, err := json.Marshal(Tool{Name: "x", Description: "d", InputSchema: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); strings.Contains(got, "annotations") {
		t.Errorf("annotations key must be omitted when nil, got %s", got)
	}
}

func TestTool_SerialisesAllHintsExplicitly(t *testing.T) {
	tool := Tool{
		Name:        "x",
		Description: "d",
		InputSchema: json.RawMessage(`{}`),
		Annotations: &ToolAnnotations{
			Title:           "Do X",
			ReadOnlyHint:    true,
			DestructiveHint: false,
			IdempotentHint:  true,
			OpenWorldHint:   false,
		},
	}
	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatal(err)
	}

	var round struct {
		Annotations map[string]any `json:"annotations"`
	}
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatal(err)
	}

	// A false hint must still be present — an omitted destructiveHint
	// means "true" per the MCP spec.
	for _, key := range []string{"title", "readOnlyHint", "destructiveHint", "idempotentHint", "openWorldHint"} {
		if _, ok := round.Annotations[key]; !ok {
			t.Errorf("annotations.%s missing from %s", key, data)
		}
	}
	if round.Annotations["destructiveHint"] != false {
		t.Errorf("destructiveHint = %v, want false", round.Annotations["destructiveHint"])
	}
	if round.Annotations["readOnlyHint"] != true {
		t.Errorf("readOnlyHint = %v, want true", round.Annotations["readOnlyHint"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race -count=1 ./internal/mcp/ -run TestTool_ -v`
Expected: FAIL — compile error `undefined: ToolAnnotations` and `unknown field Annotations in struct literal`.

- [ ] **Step 3: Write minimal implementation**

In `internal/mcp/protocol.go`, replace the `Tool` struct and add the annotations type above it:

```go
// ToolAnnotations carries the MCP behaviour hints a client uses to decide
// whether a tool may run unattended. The hint fields are serialised even
// when false: the MCP spec defaults destructiveHint to true, so an omitted
// false would be read as "destructive".
type ToolAnnotations struct {
	Title           string `json:"title"`
	ReadOnlyHint    bool   `json:"readOnlyHint"`
	DestructiveHint bool   `json:"destructiveHint"`
	IdempotentHint  bool   `json:"idempotentHint"`
	OpenWorldHint   bool   `json:"openWorldHint"`
}

type Tool struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	InputSchema json.RawMessage  `json:"inputSchema"`
	Annotations *ToolAnnotations `json:"annotations,omitempty"`
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race -count=1 ./internal/mcp/`
Expected: PASS (whole package — the existing transport and server tests must still pass).

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/protocol.go internal/mcp/annotations_test.go
git commit -m "feat(mcp): add tool annotations to the protocol types"
```

---

### Task 2: Shared schema helpers and deterministic system names

**Files:**
- Modify: `internal/zosmf/client.go:53-59`
- Modify: `internal/zosmf/ssh.go` (add a method after `Get`, around line 31)
- Create: `internal/tools/annotations.go`
- Test: `internal/tools/annotations_test.go` (create)

**Interfaces:**
- Consumes: `mcp.ToolAnnotations` from Task 1
- Produces — every later task uses these four:
  - `func (r *ClientRegistry) SystemNames() []string` — now sorted (signature unchanged, behaviour tightened)
  - `func (r *SSHClientRegistry) SystemNames() []string` — new, sorted
  - `func readOnlyTool(title string) *mcp.ToolAnnotations`
  - `func writeTool(title string, destructive, idempotent, openWorld bool) *mcp.ToolAnnotations`
  - `func systemProperty(names []string) map[string]any`
  - `func sshSystemProperty(names []string) map[string]any`

**Design note:** `ClientRegistry.SystemNames()` currently ranges over a map, so its order changes between calls. An `enum` that reshuffles on every `tools/list` invalidates client-side caching and makes tests flaky. Sorting is required, not cosmetic. Both `SystemNames` methods are currently uncalled anywhere in the repo — this task is what puts them to work.

- [ ] **Step 1: Write the failing test**

Create `internal/tools/annotations_test.go`:

```go
package tools

import (
	"reflect"
	"strings"
	"testing"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/config"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

func TestClientRegistrySystemNames_SortedAndUppercased(t *testing.T) {
	reg := zosmf.NewClientRegistry()
	for _, name := range []string{"zeta", "alpha", "Mike"} {
		client, err := zosmf.NewClient(config.SystemConfig{Name: name, Host: "https://example.invalid"})
		if err != nil {
			t.Fatal(err)
		}
		reg.Register(name, client)
	}
	want := []string{"ALPHA", "MIKE", "ZETA"}
	for i := 0; i < 5; i++ {
		if got := reg.SystemNames(); !reflect.DeepEqual(got, want) {
			t.Fatalf("SystemNames() = %v, want %v (call %d)", got, want, i)
		}
	}
}

func TestSSHClientRegistrySystemNames_SortedAndUppercased(t *testing.T) {
	reg := zosmf.NewSSHClientRegistry([]config.SSHSystemConfig{
		{Name: "zoau2"}, {Name: "Aoau1"},
	})
	want := []string{"AOAU1", "ZOAU2"}
	if got := reg.SystemNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("SystemNames() = %v, want %v", got, want)
	}
}

func TestSystemProperty_AddsEnumWhenSystemsExist(t *testing.T) {
	prop := systemProperty([]string{"PROD", "TEST"})
	if prop["type"] != "string" {
		t.Errorf("type = %v", prop["type"])
	}
	enum, ok := prop["enum"].([]string)
	if !ok || !reflect.DeepEqual(enum, []string{"PROD", "TEST"}) {
		t.Errorf("enum = %v, want [PROD TEST]", prop["enum"])
	}
	if prop["description"] == "" {
		t.Error("description must not be empty")
	}
}

func TestSystemProperty_OmitsEnumWhenNoSystems(t *testing.T) {
	prop := systemProperty(nil)
	if _, ok := prop["enum"]; ok {
		t.Error("enum must be omitted when no systems are configured — an empty enum matches nothing")
	}
}

func TestSSHSystemProperty_DescribesTheSSHNamespace(t *testing.T) {
	prop := sshSystemProperty([]string{"LPAR1"})
	desc, _ := prop["description"].(string)
	if desc == "" {
		t.Fatal("description must not be empty")
	}
	// The whole point of the separate helper: it must name ssh_systems,
	// so the model cannot confuse it with the z/OSMF systems: block.
	if !strings.Contains(desc, "ssh_systems") {
		t.Errorf("description must mention ssh_systems, got %q", desc)
	}
}

func TestReadOnlyTool_SetsHints(t *testing.T) {
	a := readOnlyTool("List Datasets")
	if a.Title != "List Datasets" {
		t.Errorf("Title = %q", a.Title)
	}
	if !a.ReadOnlyHint || a.DestructiveHint || !a.IdempotentHint || a.OpenWorldHint {
		t.Errorf("unexpected hints: %+v", a)
	}
}

func TestWriteTool_SetsHints(t *testing.T) {
	a := writeTool("Delete Dataset", true, true, false)
	if a.ReadOnlyHint {
		t.Error("ReadOnlyHint must be false for a write tool")
	}
	if !a.DestructiveHint || !a.IdempotentHint || a.OpenWorldHint {
		t.Errorf("unexpected hints: %+v", a)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race -count=1 ./internal/tools/ -run 'SystemNames|SystemProperty|Tool_SetsHints' -v`
Expected: FAIL — `reg.SystemNames undefined (type *zosmf.SSHClientRegistry has no field or method SystemNames)` and `undefined: systemProperty`.

- [ ] **Step 3: Write the implementation**

In `internal/zosmf/client.go`, replace `SystemNames`:

```go
// SystemNames returns the registered system names in sorted order.
// The order is stable so that schema enums built from it do not reshuffle
// between tools/list calls.
func (r *ClientRegistry) SystemNames() []string {
	names := make([]string, 0, len(r.clients))
	for name := range r.clients {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
```

Add `"sort"` to the imports of `client.go`.

In `internal/zosmf/ssh.go`, add after `Get` (line 31):

```go
// SystemNames returns the configured SSH system names in sorted order.
func (r *SSHClientRegistry) SystemNames() []string {
	names := make([]string, 0, len(r.systems))
	for name := range r.systems {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
```

Add `"sort"` to the imports of `ssh.go`.

Create `internal/tools/annotations.go`:

```go
package tools

import (
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
)

// readOnlyTool describes a tool that only queries state. Clients may run
// these without asking the user.
func readOnlyTool(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

// writeTool describes a tool that changes state on the z/OS system.
// destructive: the change can lose data or interrupt running work.
// idempotent: calling it twice with the same arguments has the same effect
// as calling it once.
// openWorld: the effect is not bounded by the tool's own domain — true for
// tools that pass an arbitrary command through to the system.
func writeTool(title string, destructive, idempotent, openWorld bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		ReadOnlyHint:    false,
		DestructiveHint: destructive,
		IdempotentHint:  idempotent,
		OpenWorldHint:   openWorld,
	}
}

// systemProperty builds the 'system' input schema property for tools that
// talk to z/OSMF. The enum is omitted when no systems are configured —
// an empty enum would match nothing and make every call invalid.
func systemProperty(names []string) map[string]any {
	prop := map[string]any{
		"type": "string",
		"description": "Name of the z/OS system to act on, as defined in the 'systems:' block " +
			"of the server configuration. Case-insensitive.",
	}
	if len(names) > 0 {
		prop["enum"] = names
	}
	return prop
}

// sshSystemProperty builds the 'ssh_system' property for the ZOAU tools.
// These resolve against a different configuration block than systemProperty,
// so the description spells the difference out.
func sshSystemProperty(names []string) map[string]any {
	prop := map[string]any{
		"type": "string",
		"description": "Name of the z/OS system to reach over SSH, as defined in the 'ssh_systems:' " +
			"block of the server configuration. This is a separate namespace from the 'system' " +
			"parameter of the zosmf_* tools, which resolves against 'systems:'. Case-insensitive.",
	}
	if len(names) > 0 {
		prop["enum"] = names
	}
	return prop
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race -count=1 ./internal/tools/ ./internal/zosmf/`
Expected: PASS.

- [ ] **Step 5: Verify the build and formatting**

Run: `make fmt && make vet && make build`
Expected: no output from vet, build succeeds.

- [ ] **Step 6: Commit**

```bash
git add internal/zosmf/client.go internal/zosmf/ssh.go internal/tools/annotations.go internal/tools/annotations_test.go
git commit -m "feat(tools): shared annotation and system-enum helpers"
```

---

### Task 3: Jobs and auth tools

**Files:**
- Modify: `internal/tools/jobs.go` (10 `Definition()` bodies)
- Modify: `internal/tools/auth.go` (1 `Definition()` body)
- Test: `internal/tools/definitions_test.go` (create)

**Interfaces:**
- Consumes: `readOnlyTool`, `writeTool`, `systemProperty` from Task 2
- Produces: `newTestRegistry(t)` test helper in `definitions_test.go`, reused verbatim by Tasks 4–8; and `findTool(t, reg, name) mcp.Tool`.

**The edit pattern.** In every `Definition()`, replace the inline `system` property with the helper and add the `Annotations` field. Concretely, `zosmf_list_jobs` (jobs.go:33) becomes:

```go
func (t *listJobsTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_list_jobs",
		Description: "List z/OS jobs on a system. Filter by owner, job name prefix, job ID, and/or status (ACTIVE, OUTPUT, INPUT). " +
			"To list ALL jobs (not just your own), set owner and prefix to '*'. " +
			"Set exec_data=true to include execution timestamps (exec-started, exec-ended, exec-submitted, exec-system, exec-member).",
		Annotations: readOnlyTool("List Jobs"),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": systemProperty(t.clients.SystemNames()),
				"owner": map[string]any{
					"type":        "string",
					"description": "Job owner (TSO user ID). Use '*' for all owners. Supports wildcards (e.g. 'SYS*'). Defaults to the authenticated user if omitted.",
				},
				// ... remaining properties unchanged ...
			},
			"required": []string{"system"},
		}),
	}
}
```

Note the removed `"description": "Name of the z/OS system to query (as configured)"` — `systemProperty` supplies it now. Apply the same two edits to all eleven tools in this task.

**Per-tool values.** Descriptions stay as they are unless a "Description change" is given.

| Tool | Annotations call | Description change |
|---|---|---|
| `zosmf_list_jobs` | `readOnlyTool("List Jobs")` | none |
| `zosmf_get_job` | `readOnlyTool("Get Job Details")` | none |
| `zosmf_get_job_output` | `readOnlyTool("List Job Spool Files")` | none |
| `zosmf_get_spool_content` | `readOnlyTool("Read Spool File")` | none |
| `zosmf_get_all_spools` | `readOnlyTool("Read All Spool Files")` | none |
| `zosmf_submit_job` | `writeTool("Submit Job", false, false, false)` | append: `" Returns the job name and job ID immediately — the job is not finished at that point. Poll zosmf_get_job with the returned job_name and job_id until status is OUTPUT, then read the result with zosmf_get_all_spools."` |
| `zosmf_cancel_job` | `writeTool("Cancel Job", true, true, false)` | replace with: `"Cancel a running z/OS job. WARNING: This interrupts work in progress — the job stops at an arbitrary point and may leave partial output or held resources behind. The job stays on the spool; use zosmf_purge_job to remove it."` |
| `zosmf_purge_job` | `writeTool("Purge Job", true, true, false)` | replace with: `"Purge (delete) a z/OS job's output from the spool. WARNING: Permanent — the job's spool files cannot be read afterwards. Read anything you need with zosmf_get_all_spools first."` |
| `zosmf_hold_job` | `writeTool("Hold Job", false, true, false)` | none |
| `zosmf_release_job` | `writeTool("Release Job", false, true, false)` | none |
| `zosmf_set_password` | `writeTool("Set System Password", false, true, false)` | append: `" The password is held in memory for this server process only and is not written to the configuration file — it is lost on restart."` |

- [ ] **Step 1: Write the failing test**

Create `internal/tools/definitions_test.go`:

```go
package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/config"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

// newTestRegistry builds a fully populated tool registry against two z/OSMF
// systems and one SSH system, so schema enums have something to contain.
func newTestRegistry(t *testing.T) *mcp.ToolRegistry {
	t.Helper()

	cfg := &config.Config{
		Systems: []config.SystemConfig{
			{Name: "PROD", Host: "https://prod.invalid", TSOAccount: "ACCT", TSOProc: "IZUFPROC"},
			{Name: "TEST", Host: "https://test.invalid"},
		},
		SSHSystems: []config.SSHSystemConfig{
			{Name: "LPAR1", Host: "lpar1.invalid", Username: "user", Port: 22},
		},
	}

	clients := zosmf.NewClientRegistry()
	for _, sys := range cfg.Systems {
		client, err := zosmf.NewClient(sys)
		if err != nil {
			t.Fatalf("NewClient(%s): %v", sys.Name, err)
		}
		clients.Register(sys.Name, client)
	}
	sshReg := zosmf.NewSSHClientRegistry(cfg.SSHSystems)

	reg := mcp.NewToolRegistry()
	if err := RegisterAll(reg, clients, cfg, sshReg); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	return reg
}

func findTool(t *testing.T, reg *mcp.ToolRegistry, name string) mcp.Tool {
	t.Helper()
	for _, tool := range reg.List() {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q is not registered", name)
	return mcp.Tool{}
}

// systemEnum extracts properties.<param>.enum from a tool's input schema.
func systemEnum(t *testing.T, tool mcp.Tool, param string) []string {
	t.Helper()
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
		t.Fatalf("%s: input schema is not valid JSON: %v", tool.Name, err)
	}
	prop, ok := schema.Properties[param]
	if !ok {
		t.Fatalf("%s: input schema has no %q property", tool.Name, param)
	}
	return prop.Enum
}

func assertAnnotations(t *testing.T, tool mcp.Tool, wantTitle string, readOnly, destructive, idempotent, openWorld bool) {
	t.Helper()
	a := tool.Annotations
	if a == nil {
		t.Fatalf("%s: Annotations is nil", tool.Name)
	}
	if a.Title != wantTitle {
		t.Errorf("%s: Title = %q, want %q", tool.Name, a.Title, wantTitle)
	}
	if a.ReadOnlyHint != readOnly {
		t.Errorf("%s: ReadOnlyHint = %v, want %v", tool.Name, a.ReadOnlyHint, readOnly)
	}
	if a.DestructiveHint != destructive {
		t.Errorf("%s: DestructiveHint = %v, want %v", tool.Name, a.DestructiveHint, destructive)
	}
	if a.IdempotentHint != idempotent {
		t.Errorf("%s: IdempotentHint = %v, want %v", tool.Name, a.IdempotentHint, idempotent)
	}
	if a.OpenWorldHint != openWorld {
		t.Errorf("%s: OpenWorldHint = %v, want %v", tool.Name, a.OpenWorldHint, openWorld)
	}
}

func TestJobsAndAuthTools_Annotations(t *testing.T) {
	reg := newTestRegistry(t)

	cases := []struct {
		name                                       string
		title                                      string
		readOnly, destructive, idempotent, openWorld bool
	}{
		{"zosmf_list_jobs", "List Jobs", true, false, true, false},
		{"zosmf_get_job", "Get Job Details", true, false, true, false},
		{"zosmf_get_job_output", "List Job Spool Files", true, false, true, false},
		{"zosmf_get_spool_content", "Read Spool File", true, false, true, false},
		{"zosmf_get_all_spools", "Read All Spool Files", true, false, true, false},
		{"zosmf_submit_job", "Submit Job", false, false, false, false},
		{"zosmf_cancel_job", "Cancel Job", false, true, true, false},
		{"zosmf_purge_job", "Purge Job", false, true, true, false},
		{"zosmf_hold_job", "Hold Job", false, false, true, false},
		{"zosmf_release_job", "Release Job", false, false, true, false},
		{"zosmf_set_password", "Set System Password", false, false, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := findTool(t, reg, tc.name)
			assertAnnotations(t, tool, tc.title, tc.readOnly, tc.destructive, tc.idempotent, tc.openWorld)

			enum := systemEnum(t, tool, "system")
			if len(enum) != 2 || enum[0] != "PROD" || enum[1] != "TEST" {
				t.Errorf("system enum = %v, want [PROD TEST]", enum)
			}
		})
	}
}

func TestSubmitJob_DescribesThePollingFollowUp(t *testing.T) {
	tool := findTool(t, newTestRegistry(t), "zosmf_submit_job")
	if !strings.Contains(tool.Description, "zosmf_get_job") {
		t.Errorf("submit_job must point at the polling tool, got: %q", tool.Description)
	}
}

func TestDestructiveJobTools_WarnInProse(t *testing.T) {
	reg := newTestRegistry(t)
	for _, name := range []string{"zosmf_cancel_job", "zosmf_purge_job"} {
		tool := findTool(t, reg, name)
		if !strings.Contains(tool.Description, "WARNING") {
			t.Errorf("%s: description must carry a WARNING, got: %q", name, tool.Description)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race -count=1 ./internal/tools/ -run 'JobsAndAuth|SubmitJob|DestructiveJob' -v`
Expected: FAIL — `zosmf_list_jobs: Annotations is nil`, and the enum assertions fail with `system enum = []`.

- [ ] **Step 3: Apply the edits**

Work through `internal/tools/jobs.go` and `internal/tools/auth.go` using the edit pattern and the per-tool table above. Every one of the eleven `Definition()` bodies gets an `Annotations:` field and its `system` property replaced by `systemProperty(t.clients.SystemNames())`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race -count=1 ./internal/tools/`
Expected: PASS, including the pre-existing `auth_test.go` and `datasets_test.go` tests.

- [ ] **Step 5: Commit**

```bash
git add internal/tools/jobs.go internal/tools/auth.go internal/tools/definitions_test.go
git commit -m "feat(tools): annotations and system enum for job and auth tools"
```

---

### Task 4: Dataset tools

**Files:**
- Modify: `internal/tools/datasets.go` (8 `Definition()` bodies)
- Test: `internal/tools/definitions_test.go` (append)

**Interfaces:**
- Consumes: `newTestRegistry`, `findTool`, `systemEnum`, `assertAnnotations` from Task 3
- Produces: nothing new

**Per-tool values.** Same edit pattern as Task 3.

| Tool | Annotations call | Description change |
|---|---|---|
| `zosmf_list_datasets` | `readOnlyTool("List Datasets")` | none |
| `zosmf_list_members` | `readOnlyTool("List PDS Members")` | none |
| `zosmf_read_dataset` | `readOnlyTool("Read Dataset")` | none |
| `zosmf_write_dataset` | `writeTool("Write Dataset", true, true, false)` | none — already carries its WARNING |
| `zosmf_create_dataset` | `writeTool("Allocate Dataset", false, false, false)` | replace with: `"Allocate a new z/OS dataset (sequential PS or partitioned PDS/PDSE). Fails if the dataset already exists — this is not an overwrite. Space is given in the unit selected by alcunit ('TRK' or 'CYL'); for a PDS/PDSE set dsorg to 'PO'."` |
| `zosmf_delete_dataset` | `writeTool("Delete Dataset", true, true, false)` | none — already carries its WARNING |
| `zosmf_acquire_dataset_enq` | `writeTool("Acquire Dataset ENQ", false, false, false)` | none — already the best-documented tool in the repo |
| `zosmf_release_dataset_enq` | `writeTool("Release Dataset ENQ", false, true, false)` | none |

- [ ] **Step 1: Write the failing test**

Append to `internal/tools/definitions_test.go`:

```go
func TestDatasetTools_Annotations(t *testing.T) {
	reg := newTestRegistry(t)

	cases := []struct {
		name                                       string
		title                                      string
		readOnly, destructive, idempotent, openWorld bool
	}{
		{"zosmf_list_datasets", "List Datasets", true, false, true, false},
		{"zosmf_list_members", "List PDS Members", true, false, true, false},
		{"zosmf_read_dataset", "Read Dataset", true, false, true, false},
		{"zosmf_write_dataset", "Write Dataset", false, true, true, false},
		{"zosmf_create_dataset", "Allocate Dataset", false, false, false, false},
		{"zosmf_delete_dataset", "Delete Dataset", false, true, true, false},
		{"zosmf_acquire_dataset_enq", "Acquire Dataset ENQ", false, false, false, false},
		{"zosmf_release_dataset_enq", "Release Dataset ENQ", false, false, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := findTool(t, reg, tc.name)
			assertAnnotations(t, tool, tc.title, tc.readOnly, tc.destructive, tc.idempotent, tc.openWorld)
			if enum := systemEnum(t, tool, "system"); len(enum) != 2 {
				t.Errorf("system enum = %v, want 2 entries", enum)
			}
		})
	}
}

func TestCreateDataset_SaysItFailsOnExisting(t *testing.T) {
	tool := findTool(t, newTestRegistry(t), "zosmf_create_dataset")
	if !strings.Contains(tool.Description, "already exists") {
		t.Errorf("create_dataset must state that it fails on an existing dataset, got: %q", tool.Description)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race -count=1 ./internal/tools/ -run 'DatasetTools|CreateDataset' -v`
Expected: FAIL — `zosmf_list_datasets: Annotations is nil`.

- [ ] **Step 3: Apply the edits**

Work through the eight `Definition()` bodies in `internal/tools/datasets.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race -count=1 ./internal/tools/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tools/datasets.go internal/tools/definitions_test.go
git commit -m "feat(tools): annotations and system enum for dataset tools"
```

---

### Task 5: Filesystem and USS tools

**Files:**
- Modify: `internal/tools/filesystems.go` (5 `Definition()` bodies)
- Modify: `internal/tools/ussfiles.go` (5 `Definition()` bodies)
- Test: `internal/tools/definitions_test.go` (append)

**Interfaces:**
- Consumes: the test helpers from Task 3
- Produces: nothing new

This is the task that fixes finding 4 — these ten tools carry the thinnest descriptions in the repo. Each one gets told what it returns, matching the standard set by `zosmf_list_members`.

**Per-tool values.** Descriptions here are **replacements**, not appends.

| Tool | Annotations call | New description |
|---|---|---|
| `zosmf_list_filesystems` | `readOnlyTool("List Filesystems")` | `"List mounted z/OS filesystems (zFS, HFS, etc.). Returns for each filesystem: dataset name, mount point, filesystem type, status, mount mode flags, device number, owning system name, and the read/write/directory I/O block counts. Filter by filesystem name (wildcards allowed) or by mount path."` |
| `zosmf_mount_filesystem` | `writeTool("Mount Filesystem", false, false, false)` | `"Mount a z/OS filesystem (zFS, HFS, etc.) at a USS mount point. The mount point directory must already exist. Fails if the filesystem is already mounted. Defaults to read-write ('rdwr') unless mode says otherwise."` |
| `zosmf_unmount_filesystem` | `writeTool("Unmount Filesystem", true, true, false)` | `"Unmount a mounted z/OS filesystem. WARNING: This makes the filesystem's contents inaccessible to every job and user on the system, and fails if files under the mount point are still in use."` |
| `zosmf_create_zfs` | `writeTool("Create zFS", false, false, false)` | `"Create a new zFS (z/OS File System) dataset. Allocates the backing dataset only — it is not mounted afterwards; use zosmf_mount_filesystem for that. Space is given in cylinders. Fails if the dataset already exists."` |
| `zosmf_delete_zfs` | `writeTool("Delete zFS", true, true, false)` | unchanged — already carries its WARNING |
| `zosmf_list_uss_files` | `readOnlyTool("List USS Directory")` | `"List files and directories in a z/OS Unix System Services (USS) directory. Returns for each entry: name, permission mode string, size in bytes, owning UID and user name, owning GID and group name, last modification time, and for symlinks the link target. Entries are not listed recursively."` |
| `zosmf_read_uss_file` | `readOnlyTool("Read USS File")` | `"Read the content of a z/OS USS file as text. The file is transferred with EBCDIC-to-ASCII conversion, so this is for text files — binary content will come back corrupted."` |
| `zosmf_write_uss_file` | `writeTool("Write USS File", true, true, false)` | `"Write text content to a z/OS USS file. WARNING: Overwrites the existing content — there is no append mode and no backup. The file is created if it does not exist. Content is converted from ASCII to EBCDIC."` |
| `zosmf_create_uss_file` | `writeTool("Create USS File or Directory", false, false, false)` | `"Create a new, empty z/OS USS file or directory. Fails if the path already exists. The parent directory must exist. To create a file with content in one step, use zosmf_write_uss_file instead."` |
| `zosmf_delete_uss_file` | `writeTool("Delete USS File or Directory", true, true, false)` | unchanged — already carries its WARNING |

- [ ] **Step 1: Write the failing test**

Append to `internal/tools/definitions_test.go`:

```go
func TestFilesystemAndUSSTools_Annotations(t *testing.T) {
	reg := newTestRegistry(t)

	cases := []struct {
		name                                       string
		title                                      string
		readOnly, destructive, idempotent, openWorld bool
	}{
		{"zosmf_list_filesystems", "List Filesystems", true, false, true, false},
		{"zosmf_mount_filesystem", "Mount Filesystem", false, false, false, false},
		{"zosmf_unmount_filesystem", "Unmount Filesystem", false, true, true, false},
		{"zosmf_create_zfs", "Create zFS", false, false, false, false},
		{"zosmf_delete_zfs", "Delete zFS", false, true, true, false},
		{"zosmf_list_uss_files", "List USS Directory", true, false, true, false},
		{"zosmf_read_uss_file", "Read USS File", true, false, true, false},
		{"zosmf_write_uss_file", "Write USS File", false, true, true, false},
		{"zosmf_create_uss_file", "Create USS File or Directory", false, false, false, false},
		{"zosmf_delete_uss_file", "Delete USS File or Directory", false, true, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := findTool(t, reg, tc.name)
			assertAnnotations(t, tool, tc.title, tc.readOnly, tc.destructive, tc.idempotent, tc.openWorld)
			if enum := systemEnum(t, tool, "system"); len(enum) != 2 {
				t.Errorf("system enum = %v, want 2 entries", enum)
			}
		})
	}
}

// The listing tools were one-liners that never said what came back. A model
// had to call them to find out.
func TestListingTools_EnumerateTheirReturnFields(t *testing.T) {
	reg := newTestRegistry(t)

	cases := []struct {
		name   string
		fields []string
	}{
		{"zosmf_list_uss_files", []string{"size", "UID", "group", "symlink"}},
		{"zosmf_list_filesystems", []string{"mount point", "status", "device number"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc := findTool(t, reg, tc.name).Description
			for _, field := range tc.fields {
				if !strings.Contains(desc, field) {
					t.Errorf("description must name the %q it returns, got: %q", field, desc)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race -count=1 ./internal/tools/ -run 'FilesystemAndUSS|ListingTools' -v`
Expected: FAIL — `zosmf_list_filesystems: Annotations is nil` and the return-field assertions fail against the old one-line descriptions.

- [ ] **Step 3: Apply the edits**

Work through the ten `Definition()` bodies across both files.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race -count=1 ./internal/tools/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tools/filesystems.go internal/tools/ussfiles.go internal/tools/definitions_test.go
git commit -m "feat(tools): annotations and richer descriptions for filesystem and USS tools"
```

---

### Task 6: Console, TSO, SMP/E and parmlib tools

**Files:**
- Modify: `internal/tools/console.go` (2 `Definition()` bodies)
- Modify: `internal/tools/tso.go` (1)
- Modify: `internal/tools/smpe.go` (1)
- Modify: `internal/tools/parmlib.go` (1)
- Test: `internal/tools/definitions_test.go` (append)

**Interfaces:**
- Consumes: the test helpers from Task 3
- Produces: nothing new

This task also closes finding 6: `zosmf_tso_command` fails at runtime with `system X has no tso_account configured` (tso.go:66) and `zosmf_smpe_query_csi` needs a CSI dataset and zone that may only exist in config. Neither says so up front.

**Per-tool values.**

| Tool | Annotations call | Description change |
|---|---|---|
| `zosmf_issue_command` | `writeTool("Issue Operator Command", true, false, true)` | append: `" WARNING: This is an unrestricted MVS operator console command. Commands such as 'P jobname', 'C jobname' or 'V device,OFFLINE' change or stop production work, and there is no confirmation step and no undo."` |
| `zosmf_get_hardcopy_log` | `readOnlyTool("Read Hardcopy Log")` | none |
| `zosmf_tso_command` | `writeTool("Execute TSO Command", true, false, true)` | append: `" Requires tso_account and tso_proc to be set for this system in the server configuration; without them the call fails before the command runs. WARNING: TSO commands are unrestricted — 'DELETE' and 'ALTER' reach real data."` |
| `zosmf_smpe_query_csi` | `readOnlyTool("Query SMP/E CSI")` | append: `" If csi_dataset or zone is omitted, the first entry from the system's csi_datasets / target_zones configuration is used; if the system has neither configured, the call fails and you must pass both explicitly."` |
| `zosmf_parmlib_validate` | `readOnlyTool("Validate Parmlib Member")` | none — checks syntax without applying anything |

**Note on `zosmf_issue_command`:** `idempotent` is false because operator commands like `S jobname` start a new instance on every call. `openWorld` is true because the command text is passed through unrestricted — the same reasoning applies to `zosmf_tso_command`.

- [ ] **Step 1: Write the failing test**

Append to `internal/tools/definitions_test.go`:

```go
func TestConsoleTSOAndInventoryTools_Annotations(t *testing.T) {
	reg := newTestRegistry(t)

	cases := []struct {
		name                                       string
		title                                      string
		readOnly, destructive, idempotent, openWorld bool
	}{
		{"zosmf_issue_command", "Issue Operator Command", false, true, false, true},
		{"zosmf_get_hardcopy_log", "Read Hardcopy Log", true, false, true, false},
		{"zosmf_tso_command", "Execute TSO Command", false, true, false, true},
		{"zosmf_smpe_query_csi", "Query SMP/E CSI", true, false, true, false},
		{"zosmf_parmlib_validate", "Validate Parmlib Member", true, false, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := findTool(t, reg, tc.name)
			assertAnnotations(t, tool, tc.title, tc.readOnly, tc.destructive, tc.idempotent, tc.openWorld)
			if enum := systemEnum(t, tool, "system"); len(enum) != 2 {
				t.Errorf("system enum = %v, want 2 entries", enum)
			}
		})
	}
}

// Finding 6: these preconditions used to surface only as a runtime error.
func TestToolsDeclareTheirConfigPreconditions(t *testing.T) {
	reg := newTestRegistry(t)

	cases := []struct {
		name string
		keys []string
	}{
		{"zosmf_tso_command", []string{"tso_account", "tso_proc"}},
		{"zosmf_smpe_query_csi", []string{"csi_datasets", "target_zones"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc := findTool(t, reg, tc.name).Description
			for _, key := range tc.keys {
				if !strings.Contains(desc, key) {
					t.Errorf("description must name the required config key %q, got: %q", key, desc)
				}
			}
		})
	}
}

// The two tools that pass an arbitrary command through to the system must
// be flagged as open-world, or a client will treat them as bounded.
func TestArbitraryCommandTools_AreOpenWorld(t *testing.T) {
	reg := newTestRegistry(t)
	for _, name := range []string{"zosmf_issue_command", "zosmf_tso_command"} {
		tool := findTool(t, reg, name)
		if !tool.Annotations.OpenWorldHint {
			t.Errorf("%s: OpenWorldHint must be true", name)
		}
		if !strings.Contains(tool.Description, "WARNING") {
			t.Errorf("%s: description must carry a WARNING, got: %q", name, tool.Description)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race -count=1 ./internal/tools/ -run 'ConsoleTSO|ConfigPreconditions|ArbitraryCommand' -v`
Expected: FAIL — `zosmf_issue_command: Annotations is nil`.

- [ ] **Step 3: Apply the edits**

Work through the five `Definition()` bodies across the four files.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race -count=1 ./internal/tools/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tools/console.go internal/tools/tso.go internal/tools/smpe.go internal/tools/parmlib.go internal/tools/definitions_test.go
git commit -m "feat(tools): annotations and config preconditions for console, TSO, SMP/E and parmlib tools"
```

---

### Task 7: ZOAU namespace rename

**Files:**
- Modify: `internal/tools/zoau.go` (11 `Definition()` bodies, 11 constructor names, 11 arg structs)
- Modify: `internal/tools/registry.go:59-71`
- Test: `internal/tools/definitions_test.go` (append)

**Interfaces:**
- Consumes: `sshSystemProperty`, `readOnlyTool`, `writeTool` from Task 2; the test helpers from Task 3
- Produces: renamed constructors `NewZOAU*Tool` keep their Go names — only the MCP-visible tool names and the JSON parameter change.

**This task is the breaking change.** Three edits per tool:

1. `Name: "zosmf_zoau_pll"` → `Name: "zoau_pll"`
2. The `"system"` schema property becomes `"ssh_system": sshSystemProperty(t.sshReg.SystemNames())`, and `"required"` changes from `[]string{"system"}` to `[]string{"ssh_system"}`
3. The arg struct's JSON tag changes: `System string \`json:"system"\`` → `SSHSystem string \`json:"ssh_system"\``, and every `args.System` reference in `Execute()` becomes `args.SSHSystem`

Do **not** add any alias registration. The old names must disappear from `tools/list`.

**Per-tool values.** All ZOAU tools except `zoau_exec` are read-only queries.

| Old name | New name | Annotations call |
|---|---|---|
| `zosmf_zoau_pll` | `zoau_pll` | `readOnlyTool("Show Linklist")` |
| `zosmf_zoau_zinfo` | `zoau_zinfo` | `readOnlyTool("Show System Information")` |
| `zosmf_zoau_llwhence` | `zoau_llwhence` | `readOnlyTool("Locate Linklist Member")` |
| `zosmf_zoau_parmgrep` | `zoau_parmgrep` | `readOnlyTool("Search Parmlib")` |
| `zosmf_zoau_parmwhence` | `zoau_parmwhence` | `readOnlyTool("Locate Parmlib Member")` |
| `zosmf_zoau_pproc` | `zoau_pproc` | `readOnlyTool("Show Proclib")` |
| `zosmf_zoau_procgrep` | `zoau_procgrep` | `readOnlyTool("Search Proclib")` |
| `zosmf_zoau_procwhence` | `zoau_procwhence` | `readOnlyTool("Locate Proclib Member")` |
| `zosmf_zoau_vtocls` | `zoau_vtocls` | `readOnlyTool("List Volume VTOC")` |
| `zosmf_zoau_version` | `zoau_version` | `readOnlyTool("Show ZOAU Version")` |
| `zosmf_zoau_exec` | `zoau_exec` | `writeTool("Execute USS Command", true, false, true)` |

**Finding 5 — `zoau_exec` description.** Replace it entirely with:

```go
Description: "Execute an arbitrary command or script on a z/OS USS system over SSH. " +
	"Returns stdout, stderr and the exit code as JSON. Suitable for REXX scripts, shell scripts, " +
	"Python scripts, and any USS command. " +
	"WARNING: This is unrestricted shell access under the configured SSH user's authority — it can " +
	"delete data, submit jobs and change system state, with no confirmation step and no undo. " +
	"Prefer a specific tool when one exists. " +
	"The command string and each entry of args are shell-quoted individually, so a value containing " +
	"spaces or quotes stays one argument; the command itself is not restricted to a whitelist.",
```

- [ ] **Step 1: Write the failing test**

Append to `internal/tools/definitions_test.go`:

```go
func TestZOAUTools_UseTheirOwnNamespace(t *testing.T) {
	reg := newTestRegistry(t)

	names := make(map[string]bool)
	for _, tool := range reg.List() {
		names[tool.Name] = true
	}

	for _, old := range []string{
		"zosmf_zoau_pll", "zosmf_zoau_zinfo", "zosmf_zoau_llwhence", "zosmf_zoau_parmgrep",
		"zosmf_zoau_parmwhence", "zosmf_zoau_pproc", "zosmf_zoau_procgrep", "zosmf_zoau_procwhence",
		"zosmf_zoau_vtocls", "zosmf_zoau_version", "zosmf_zoau_exec",
	} {
		if names[old] {
			t.Errorf("legacy tool name %q is still registered — the rename is deliberate and has no alias", old)
		}
	}

	for _, want := range []string{
		"zoau_pll", "zoau_zinfo", "zoau_llwhence", "zoau_parmgrep", "zoau_parmwhence",
		"zoau_pproc", "zoau_procgrep", "zoau_procwhence", "zoau_vtocls", "zoau_version", "zoau_exec",
	} {
		if !names[want] {
			t.Errorf("tool %q is not registered", want)
		}
	}
}

func TestZOAUTools_AnnotationsAndSSHSystemEnum(t *testing.T) {
	reg := newTestRegistry(t)

	readOnly := []struct{ name, title string }{
		{"zoau_pll", "Show Linklist"},
		{"zoau_zinfo", "Show System Information"},
		{"zoau_llwhence", "Locate Linklist Member"},
		{"zoau_parmgrep", "Search Parmlib"},
		{"zoau_parmwhence", "Locate Parmlib Member"},
		{"zoau_pproc", "Show Proclib"},
		{"zoau_procgrep", "Search Proclib"},
		{"zoau_procwhence", "Locate Proclib Member"},
		{"zoau_vtocls", "List Volume VTOC"},
		{"zoau_version", "Show ZOAU Version"},
	}

	for _, tc := range readOnly {
		t.Run(tc.name, func(t *testing.T) {
			tool := findTool(t, reg, tc.name)
			assertAnnotations(t, tool, tc.title, true, false, true, false)

			enum := systemEnum(t, tool, "ssh_system")
			if len(enum) != 1 || enum[0] != "LPAR1" {
				t.Errorf("ssh_system enum = %v, want [LPAR1]", enum)
			}
		})
	}

	exec := findTool(t, reg, "zoau_exec")
	assertAnnotations(t, exec, "Execute USS Command", false, true, false, true)
	if enum := systemEnum(t, exec, "ssh_system"); len(enum) != 1 {
		t.Errorf("zoau_exec ssh_system enum = %v, want 1 entry", enum)
	}
	if !strings.Contains(exec.Description, "WARNING") {
		t.Errorf("zoau_exec must carry a WARNING, got: %q", exec.Description)
	}
	if !strings.Contains(exec.Description, "unrestricted") {
		t.Errorf("zoau_exec must say the access is unrestricted, got: %q", exec.Description)
	}
}

// No ZOAU tool may keep a 'system' parameter — that name belongs to the
// z/OSMF namespace and having both is what confused callers.
func TestZOAUTools_HaveNoSystemParameter(t *testing.T) {
	for _, tool := range newTestRegistry(t).List() {
		if len(tool.Name) < 5 || tool.Name[:5] != "zoau_" {
			continue
		}
		var schema struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("%s: %v", tool.Name, err)
		}
		if _, ok := schema.Properties["system"]; ok {
			t.Errorf("%s: still has a 'system' property, want 'ssh_system'", tool.Name)
		}
		if _, ok := schema.Properties["ssh_system"]; !ok {
			t.Errorf("%s: missing 'ssh_system' property", tool.Name)
		}
		found := false
		for _, r := range schema.Required {
			if r == "ssh_system" {
				found = true
			}
			if r == "system" {
				t.Errorf("%s: 'system' is still listed as required", tool.Name)
			}
		}
		if !found {
			t.Errorf("%s: 'ssh_system' must be required", tool.Name)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race -count=1 ./internal/tools/ -run ZOAUTools -v`
Expected: FAIL — `legacy tool name "zosmf_zoau_pll" is still registered`, `tool "zoau_pll" is not registered`.

- [ ] **Step 3: Apply the edits**

Work through all eleven tools in `internal/tools/zoau.go`. Each needs the name change, the `ssh_system` property, the `required` change, the arg-struct JSON tag change, the `Execute()` field rename, and the annotations. Check `registry.go` still compiles — the Go constructor names do not change, so it should need no edit; verify rather than assume.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race -count=1 ./internal/tools/ && make vet && make build`
Expected: PASS, no vet output, build succeeds.

- [ ] **Step 5: Commit**

```bash
git add internal/tools/zoau.go internal/tools/registry.go internal/tools/definitions_test.go
git commit -m "feat(zoau)!: move ZOAU tools to the zoau_ namespace with an ssh_system parameter

BREAKING CHANGE: the eleven zosmf_zoau_* tools are renamed to zoau_* and
their 'system' parameter is renamed to 'ssh_system'. There is no alias for
the old names. Clients that call the old names — n8n workflows, agent
prompts, saved tool configurations — must be updated."
```

---

### Task 8: Completeness gate and documentation

**Files:**
- Test: `internal/tools/definitions_test.go` (append)
- Modify: `README.md`
- Modify: `.zosmf-mcp-config.example.yaml`

**Interfaces:**
- Consumes: everything from Tasks 1–7
- Produces: the invariant that catches any future tool added without annotations

The per-domain tests above each cover their own tools. This task adds the catch-all so a tool added next year cannot slip through, and documents the breaking rename for anyone upgrading.

- [ ] **Step 1: Write the failing test**

Append to `internal/tools/definitions_test.go`:

```go
// Every registered tool must be fully described. This is the gate that
// catches a new tool added without annotations or without a system enum.
func TestAllTools_AreFullyDescribed(t *testing.T) {
	reg := newTestRegistry(t)
	tools := reg.List()

	if len(tools) != 45 {
		t.Errorf("registered %d tools, expected 45 — update this count deliberately when adding one", len(tools))
	}

	for _, tool := range tools {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.Annotations == nil {
				t.Fatal("Annotations is nil")
			}
			if tool.Annotations.Title == "" {
				t.Error("Annotations.Title is empty")
			}
			if tool.Annotations.ReadOnlyHint && tool.Annotations.DestructiveHint {
				t.Error("a read-only tool cannot be destructive")
			}

			if len(tool.Description) < 60 {
				t.Errorf("description is too thin (%d chars): %q", len(tool.Description), tool.Description)
			}

			var schema struct {
				Properties map[string]struct {
					Type        string   `json:"type"`
					Description string   `json:"description"`
					Enum        []string `json:"enum"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
				t.Fatalf("input schema is not valid JSON: %v", err)
			}

			for name, prop := range schema.Properties {
				if prop.Description == "" {
					t.Errorf("property %q has no description", name)
				}
			}

			// Exactly one of the two system parameters, and it must be enumerated.
			_, hasSystem := schema.Properties["system"]
			_, hasSSHSystem := schema.Properties["ssh_system"]
			switch {
			case hasSystem && hasSSHSystem:
				t.Error("has both 'system' and 'ssh_system'")
			case hasSystem:
				if len(schema.Properties["system"].Enum) == 0 {
					t.Error("'system' has no enum")
				}
			case hasSSHSystem:
				if len(schema.Properties["ssh_system"].Enum) == 0 {
					t.Error("'ssh_system' has no enum")
				}
			default:
				t.Error("has neither 'system' nor 'ssh_system'")
			}
		})
	}
}

// A server with no systems configured must still produce a valid schema —
// an empty enum would match nothing and make every call impossible.
func TestTools_OmitEnumWhenNothingIsConfigured(t *testing.T) {
	reg := mcp.NewToolRegistry()
	if err := RegisterAll(reg, zosmf.NewClientRegistry(), &config.Config{}, nil); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	for _, tool := range reg.List() {
		var schema struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("%s: %v", tool.Name, err)
		}
		if prop, ok := schema.Properties["system"]; ok && prop.Enum != nil {
			t.Errorf("%s: enum must be absent when no systems are configured, got %v", tool.Name, prop.Enum)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails or reveals gaps**

Run: `go test -race -count=1 ./internal/tools/ -run 'AllTools|OmitEnum' -v`
Expected: PASS if Tasks 3–7 were complete. If it FAILS, the failure names the tool and the missing piece — fix that tool in this task rather than starting a new one. Do not weaken the assertions to make them pass.

- [ ] **Step 3: Update the README**

In `README.md`, find the tool listing and apply:

1. Rename all eleven `zosmf_zoau_*` entries to `zoau_*`.
2. In the ZOAU section, add a note directly under the heading:

```markdown
> **Breaking change:** As of this version the ZOAU tools are named `zoau_*`
> (previously `zosmf_zoau_*`) and take an `ssh_system` parameter (previously
> `system`). The old names are no longer registered — update any n8n workflow,
> agent prompt or saved client configuration that calls them.
>
> `ssh_system` resolves against the `ssh_systems:` config block; the `system`
> parameter of the `zosmf_*` tools resolves against `systems:`. They are
> separate namespaces.
```

3. Add a short subsection explaining the annotations, so operators understand why some tools now prompt:

```markdown
### Tool annotations

Every tool carries MCP annotations (`readOnlyHint`, `destructiveHint`,
`idempotentHint`, `openWorldHint`). Clients use them to decide what may run
unattended. Read-only tools such as `zosmf_list_datasets` are safe to
auto-approve; tools marked destructive — `zosmf_delete_dataset`,
`zosmf_write_dataset`, `zosmf_purge_job`, `zosmf_cancel_job`,
`zosmf_unmount_filesystem`, `zosmf_delete_zfs`, `zosmf_delete_uss_file`,
`zosmf_write_uss_file`, `zosmf_issue_command`, `zosmf_tso_command` and
`zoau_exec` — change or destroy state and should stay behind a confirmation.

The `system` parameter of each tool is enumerated from your configuration, so
a client sees the systems you actually have instead of guessing a name.
```

- [ ] **Step 4: Update the example config**

In `.zosmf-mcp-config.example.yaml`, check the comment above the `ssh_systems:` block. If it references the ZOAU tools by their old names, update them to `zoau_*`. If it does not mention tool names at all, add one line:

```yaml
# Systems reachable over SSH for the zoau_* tools. The 'ssh_system' parameter
# of those tools refers to the names defined here, not to the 'systems:' block.
```

- [ ] **Step 5: Full verification**

Run: `make fmt && make vet && make test && make build`
Expected: all green. Then confirm the working tree carries no unintended change: `git status --short`.

- [ ] **Step 6: Commit**

```bash
git add internal/tools/definitions_test.go README.md .zosmf-mcp-config.example.yaml
git commit -m "test(tools): gate every tool on annotations and a system enum, document the ZOAU rename"
```

---

## Self-Review Notes

**Spec coverage** — all six findings map to tasks:

| Finding | Task |
|---|---|
| 1. `system` without enum | 2 (helper), 3–7 (applied) |
| 2. No annotations | 1 (type), 3–7 (applied) |
| 3. ZOAU namespace | 7 |
| 4. Inconsistent description depth | 5 (the thin ones), 4 and 6 (targeted additions) |
| 5. `zoau_exec` underdescribed | 7 |
| 6. Invisible preconditions | 6 (`tso_account`/`tso_proc`, CSI zones), 3 (`submit_job` polling) |

**Known risk:** `TestAllTools_AreFullyDescribed` asserts a tool count of 45. That number is deliberate — it forces a reviewer to look when the count changes. If Task 7's rename accidentally drops or duplicates a tool, this test catches it.

**Not in scope:** `outputSchema` per tool. All tools return `json.MarshalIndent` blobs of the z/OSMF response, so a faithful output schema would mean transcribing the z/OSMF data model — a much larger job with its own design questions. Worth a separate spec if the client benefit turns out to be real.
