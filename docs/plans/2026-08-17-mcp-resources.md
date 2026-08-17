# MCP Resources and resource_link Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give datasets, USS files, job spool files and the system inventory addressable MCP resource URIs, and stop large read results from flooding the model's context by returning `resource_link` above a size threshold.

**Architecture:** A new package `internal/resources/` mirrors the existing `internal/tools/` layout — one file per object type, a registry that routes by URI, and a shared `uri.go` that `internal/tools/` imports to build links. `internal/mcp/` gains the protocol types and three dispatch branches. Tools and `resources/read` both call the same `internal/zosmf/` functions; only the packaging differs.

**Tech Stack:** Go 1.26, standard library only (`google/uuid` and `gopkg.in/yaml.v3` are the sole dependencies and neither is needed here). Tests use `net/http/httptest`.

**Design doc:** `docs/2026-08-17-mcp-resources-design.md`

## Global Constraints

- MCP protocol version is `2025-06-18` (raised from `2025-03-26`).
- No new third-party dependencies.
- `resources.inline_max_bytes` defaults to `32768`.
- Preview size is a constant `2048` bytes, not configurable.
- Hard ceiling on `resources/read` is a constant `10 * 1024 * 1024`, not configurable.
- Dataset names are upper-cased when parsed; USS paths are left untouched.
- No caching, no subscriptions, `listChanged` stays off.
- Build and test only via `make` — `make test`, `make build`, `make check-identifiers`.
- Commit messages: short. Subject line, body only when the why is not obvious.
- Do NOT add a `Co-Authored-By` trailer.
- All code, comments and docs in English.

## File Structure

**Create:**

| File | Responsibility |
| --- | --- |
| `internal/resources/uri.go` | Parse and build `zosmf://` URIs. Pure functions, no I/O. |
| `internal/resources/uri_test.go` | Table-driven round-trip tests. |
| `internal/resources/registry.go` | `ResourceHandler` interface, `ResourceRegistry`, `RegisterAll`. |
| `internal/resources/registry_test.go` | Registration, duplicates, routing, unknown URI. |
| `internal/resources/systems.go` | `zosmf:///systems` — the one listable resource. |
| `internal/resources/systems_test.go` | |
| `internal/resources/datasets.go` | `zosmf://{system}/dataset/{dsn}` |
| `internal/resources/datasets_test.go` | |
| `internal/resources/ussfiles.go` | `zosmf://{system}/uss/{path}` |
| `internal/resources/ussfiles_test.go` | |
| `internal/resources/jobspool.go` | `zosmf://{system}/job/{jobname}/{jobid}/spool/{id}` |
| `internal/resources/jobspool_test.go` | |
| `internal/tools/largecontent.go` | Threshold, preview, budget helpers shared by the read tools. |
| `internal/tools/largecontent_test.go` | |

**Modify:**

| File | Change |
| --- | --- |
| `internal/mcp/protocol.go` | Version bump, resource types, `Content` extension, `ErrCodeResourceNotFound`. |
| `internal/mcp/server.go` | Resources capability, three dispatch branches, handlers. |
| `internal/config/config.go` | `ResourcesConfig` with `inline_max_bytes`, default applied. |
| `internal/tools/datasets.go` | `zosmf_read_dataset` emits preview + link above threshold. |
| `internal/tools/ussfiles.go` | `zosmf_read_uss_file` likewise. |
| `internal/tools/jobs.go` | `zosmf_get_spool_content` likewise; `zosmf_get_all_spools` budget logic. |
| `internal/tools/registry.go` | `RegisterAll` takes the threshold. |
| `cmd/zosmf-mcp/main.go` | Build the resource registry, pass it to the server. |
| `.zosmf-mcp-config.example.yaml` | Document `resources.inline_max_bytes`. |
| `README.md` | Resources section. |

---

### Task 1: Protocol types and version bump

**Files:**
- Modify: `internal/mcp/protocol.go`
- Modify: `internal/mcp/server.go:182` (capabilities in `handleInitialize`)
- Test: `internal/mcp/resources_protocol_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: `mcp.Resource`, `mcp.ResourceTemplate`, `mcp.ResourceContents`, `mcp.ResourcesCapability`, `mcp.ResourcesListResult`, `mcp.ResourceTemplatesListResult`, `mcp.ResourcesReadParams`, `mcp.ResourcesReadResult`, `mcp.ResourceLink(uri, name, description, mimeType string, size int64) Content`, `mcp.ErrCodeResourceNotFound`.

- [ ] **Step 1: Write the failing test**

Create `internal/mcp/resources_protocol_test.go`:

```go
package mcp

import (
	"encoding/json"
	"log"
	"os"
	"testing"
)

func TestInitializeAdvertisesResources(t *testing.T) {
	s := NewServer(NewToolRegistry(), log.New(os.Stderr, "", 0))

	params, _ := json.Marshal(InitializeParams{
		ProtocolVersion: "2025-06-18",
		ClientInfo:      Implementation{Name: "test", Version: "1"},
	})
	msg := &JSONRPCMessage{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "initialize", Params: params}

	resp := s.HandleMessage(nil, msg)
	if resp.Error != nil {
		t.Fatalf("initialize failed: %v", resp.Error)
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ProtocolVersion != "2025-06-18" {
		t.Errorf("ProtocolVersion = %q, want 2025-06-18", result.ProtocolVersion)
	}
	if result.Capabilities.Resources == nil {
		t.Fatal("Capabilities.Resources is nil, want advertised")
	}
	if result.Capabilities.Resources.Subscribe {
		t.Error("Subscribe = true, want false")
	}
}

func TestInitializeAcceptsOlderProtocolVersion(t *testing.T) {
	s := NewServer(NewToolRegistry(), log.New(os.Stderr, "", 0))

	params, _ := json.Marshal(InitializeParams{
		ProtocolVersion: "2025-03-26",
		ClientInfo:      Implementation{Name: "old-client", Version: "1"},
	})
	msg := &JSONRPCMessage{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "initialize", Params: params}

	resp := s.HandleMessage(nil, msg)
	if resp.Error != nil {
		t.Fatalf("initialize with older protocol version failed: %v", resp.Error)
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ProtocolVersion != "2025-06-18" {
		t.Errorf("ProtocolVersion = %q, want the version the server speaks", result.ProtocolVersion)
	}
}

func TestResourceLinkMarshalling(t *testing.T) {
	c := ResourceLink("zosmf://S0W1/dataset/USER.DATA", "USER.DATA", "sequential dataset", "text/plain", 4096)

	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["type"] != "resource_link" {
		t.Errorf("type = %v, want resource_link", got["type"])
	}
	if got["uri"] != "zosmf://S0W1/dataset/USER.DATA" {
		t.Errorf("uri = %v", got["uri"])
	}
	if got["size"] != float64(4096) {
		t.Errorf("size = %v, want 4096", got["size"])
	}
	if _, present := got["text"]; present {
		t.Error("text must be omitted on a resource_link")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/ -run 'Resource|Initialize' -v`
Expected: FAIL — `result.Capabilities.Resources` undefined, `ResourceLink` undefined.

- [ ] **Step 3: Write the implementation**

In `internal/mcp/protocol.go`, change line 5:

```go
const ProtocolVersion = "2025-06-18"
```

Extend `ServerCapabilities` and add the resource types:

```go
type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
}

// ResourcesCapability advertises resource support. Both flags stay false:
// the resource inventory is static, so there is nothing to subscribe to and
// nothing that changes.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// Resource is a concrete, listable resource.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

// ResourceTemplate describes a family of resources whose members cannot be
// enumerated — every dataset on a system, for instance.
type ResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
}

// ResourceContents carries the body of one resource. Text and Blob are
// mutually exclusive; Blob is base64 and used when the content is not valid
// UTF-8.
type ResourceContents struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

type ResourcesListResult struct {
	Resources []Resource `json:"resources"`
}

type ResourceTemplatesListResult struct {
	ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
}

type ResourcesReadParams struct {
	URI string `json:"uri"`
}

type ResourcesReadResult struct {
	Contents []ResourceContents `json:"contents"`
}
```

Extend `Content` with the `resource_link` fields and add the constructor:

```go
type Content struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Data        string `json:"data,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
	URI         string `json:"uri,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

// ResourceLink returns a pointer to a resource instead of its content. Used
// when a result would otherwise be too large to put in the model's context.
func ResourceLink(uri, name, description, mimeType string, size int64) Content {
	return Content{
		Type:        "resource_link",
		URI:         uri,
		Name:        name,
		Description: description,
		MIMEType:    mimeType,
		Size:        size,
	}
}
```

Add the error code next to the existing `ErrCode*` constants:

```go
// ErrCodeResourceNotFound is the MCP-specific code for a resource URI that
// does not resolve.
const ErrCodeResourceNotFound = -32002
```

In `internal/mcp/server.go`, extend the capabilities in `handleInitialize` (around line 182):

```go
		Capabilities: ServerCapabilities{
			Tools:     &ToolsCapability{},
			Resources: &ResourcesCapability{},
		},
```

Keep whatever the existing `Tools` value is; only add the `Resources` line.

- [ ] **Step 4: Run tests**

Run: `make test`
Expected: PASS, all packages.

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/
git commit -m "feat(mcp): add resource protocol types, raise version to 2025-06-18"
```

---

### Task 2: URI parsing and building

**Files:**
- Create: `internal/resources/uri.go`
- Test: `internal/resources/uri_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `resources.Kind` (`KindSystems`, `KindDataset`, `KindUSS`, `KindSpool`), `resources.Ref`, `resources.ParseURI(string) (Ref, error)`, `resources.SystemsURI`, `resources.DatasetURI(system, dsn string) string`, `resources.USSURI(system, path string) string`, `resources.SpoolURI(system, jobName, jobID string, fileID int) string`.

- [ ] **Step 1: Write the failing test**

Create `internal/resources/uri_test.go`:

```go
package resources

import "testing"

func TestParseURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want Ref
	}{
		{
			name: "systems inventory",
			uri:  "zosmf:///systems",
			want: Ref{Kind: KindSystems},
		},
		{
			name: "sequential dataset",
			uri:  "zosmf://S0W1/dataset/USER.DATA.SEQ",
			want: Ref{Kind: KindDataset, System: "S0W1", Dataset: "USER.DATA.SEQ"},
		},
		{
			name: "pds member",
			uri:  "zosmf://S0W1/dataset/SYS1.PARMLIB(IEASYS00)",
			want: Ref{Kind: KindDataset, System: "S0W1", Dataset: "SYS1.PARMLIB(IEASYS00)"},
		},
		{
			name: "dataset lower case is upper-cased",
			uri:  "zosmf://s0w1/dataset/user.data",
			want: Ref{Kind: KindDataset, System: "S0W1", Dataset: "USER.DATA"},
		},
		{
			name: "uss file",
			uri:  "zosmf://S0W1/uss/etc/profile",
			want: Ref{Kind: KindUSS, System: "S0W1", Path: "/etc/profile"},
		},
		{
			name: "uss path keeps its case",
			uri:  "zosmf://S0W1/uss/u/ibmuser/MyScript.sh",
			want: Ref{Kind: KindUSS, System: "S0W1", Path: "/u/ibmuser/MyScript.sh"},
		},
		{
			name: "spool file",
			uri:  "zosmf://S0W1/job/MYJOB/JOB12345/spool/2",
			want: Ref{Kind: KindSpool, System: "S0W1", JobName: "MYJOB", JobID: "JOB12345", FileID: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseURI(tt.uri)
			if err != nil {
				t.Fatalf("ParseURI(%q) returned error: %v", tt.uri, err)
			}
			if got != tt.want {
				t.Errorf("ParseURI(%q) = %+v, want %+v", tt.uri, got, tt.want)
			}
		})
	}
}

func TestParseURIRejects(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{"unknown scheme", "http://S0W1/dataset/USER.DATA"},
		{"unknown kind", "zosmf://S0W1/volume/VOL001"},
		{"dataset without name", "zosmf://S0W1/dataset/"},
		{"uss without path", "zosmf://S0W1/uss"},
		{"spool with non-numeric id", "zosmf://S0W1/job/MYJOB/JOB12345/spool/abc"},
		{"spool truncated", "zosmf://S0W1/job/MYJOB/JOB12345"},
		{"system-scoped without system", "zosmf:///dataset/USER.DATA"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseURI(tt.uri); err == nil {
				t.Errorf("ParseURI(%q) succeeded, want error", tt.uri)
			}
		})
	}
}

func TestBuildersRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want Ref
	}{
		{"dataset", DatasetURI("S0W1", "SYS1.PARMLIB(IEASYS00)"),
			Ref{Kind: KindDataset, System: "S0W1", Dataset: "SYS1.PARMLIB(IEASYS00)"}},
		{"uss", USSURI("S0W1", "/etc/profile"),
			Ref{Kind: KindUSS, System: "S0W1", Path: "/etc/profile"}},
		{"spool", SpoolURI("S0W1", "MYJOB", "JOB12345", 2),
			Ref{Kind: KindSpool, System: "S0W1", JobName: "MYJOB", JobID: "JOB12345", FileID: 2}},
		{"systems", SystemsURI, Ref{Kind: KindSystems}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseURI(tt.uri)
			if err != nil {
				t.Fatalf("ParseURI(%q): %v", tt.uri, err)
			}
			if got != tt.want {
				t.Errorf("round trip of %q = %+v, want %+v", tt.uri, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/resources/ -v`
Expected: FAIL — package does not compile, `Ref` and `ParseURI` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/resources/uri.go`:

```go
// Package resources implements MCP resources for z/OS objects. Datasets,
// USS files and job spool files get addressable URIs so a client can attach
// them as context without a tool call, and so tools can return a pointer
// instead of a payload when the content is large.
package resources

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind is the type of object a URI refers to.
type Kind int

const (
	KindSystems Kind = iota
	KindDataset
	KindUSS
	KindSpool
)

// SystemsURI addresses the server-level system inventory. It uses an empty
// authority so it cannot collide with a system actually named SYSTEMS.
const SystemsURI = "zosmf:///systems"

const scheme = "zosmf://"

// Ref is a parsed resource URI. Which fields are set depends on Kind.
type Ref struct {
	Kind    Kind
	System  string
	Dataset string
	Path    string
	JobName string
	JobID   string
	FileID  int
}

// DatasetURI builds the URI for a dataset or PDS member.
func DatasetURI(system, dsn string) string {
	return fmt.Sprintf("%s%s/dataset/%s", scheme, strings.ToUpper(system), strings.ToUpper(dsn))
}

// USSURI builds the URI for a USS file. path must be absolute.
func USSURI(system, path string) string {
	return fmt.Sprintf("%s%s/uss%s", scheme, strings.ToUpper(system), path)
}

// SpoolURI builds the URI for one spool file of a job.
func SpoolURI(system, jobName, jobID string, fileID int) string {
	return fmt.Sprintf("%s%s/job/%s/%s/spool/%d",
		scheme, strings.ToUpper(system), strings.ToUpper(jobName), strings.ToUpper(jobID), fileID)
}

// ParseURI turns a zosmf:// URI into a Ref. Dataset and job identifiers are
// upper-cased, since z/OS treats them case-insensitively; USS paths are left
// alone, where case is significant.
func ParseURI(uri string) (Ref, error) {
	if uri == SystemsURI {
		return Ref{Kind: KindSystems}, nil
	}
	if !strings.HasPrefix(uri, scheme) {
		return Ref{}, fmt.Errorf("not a zosmf resource URI: %q", uri)
	}

	rest := strings.TrimPrefix(uri, scheme)
	system, rest, found := strings.Cut(rest, "/")
	if !found || system == "" {
		return Ref{}, fmt.Errorf("missing system in URI: %q", uri)
	}
	system = strings.ToUpper(system)

	switch {
	case strings.HasPrefix(rest, "dataset/"):
		dsn := strings.TrimPrefix(rest, "dataset/")
		if dsn == "" {
			return Ref{}, fmt.Errorf("missing dataset name in URI: %q", uri)
		}
		return Ref{Kind: KindDataset, System: system, Dataset: strings.ToUpper(dsn)}, nil

	case strings.HasPrefix(rest, "uss/"):
		path := strings.TrimPrefix(rest, "uss")
		if path == "" || path == "/" {
			return Ref{}, fmt.Errorf("missing USS path in URI: %q", uri)
		}
		return Ref{Kind: KindUSS, System: system, Path: path}, nil

	case strings.HasPrefix(rest, "job/"):
		parts := strings.Split(strings.TrimPrefix(rest, "job/"), "/")
		if len(parts) != 4 || parts[2] != "spool" {
			return Ref{}, fmt.Errorf("malformed job spool URI: %q", uri)
		}
		id, err := strconv.Atoi(parts[3])
		if err != nil {
			return Ref{}, fmt.Errorf("spool file id is not numeric in URI: %q", uri)
		}
		return Ref{
			Kind:    KindSpool,
			System:  system,
			JobName: strings.ToUpper(parts[0]),
			JobID:   strings.ToUpper(parts[1]),
			FileID:  id,
		}, nil
	}

	return Ref{}, fmt.Errorf("unknown resource kind in URI: %q", uri)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/resources/ -v`
Expected: PASS, all subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/resources/
git commit -m "feat(resources): add zosmf URI parsing and builders"
```

---

### Task 3: Registry and server dispatch

**Files:**
- Create: `internal/resources/registry.go`
- Test: `internal/resources/registry_test.go`
- Modify: `internal/mcp/server.go`

**Interfaces:**
- Consumes: `resources.ParseURI`, `mcp.Resource`, `mcp.ResourceTemplate`, `mcp.ResourceContents`, `mcp.ErrCodeResourceNotFound`.
- Produces: `resources.ResourceHandler` interface, `resources.ResourceRegistry` with `Register(ResourceHandler) error`, `List() []mcp.Resource`, `Templates() []mcp.ResourceTemplate`, `Read(ctx, uri string) (mcp.ResourceContents, error)`, and `resources.ErrNotFound`. Server gains `mcp.Server.SetResourceProvider(ResourceProvider)`.

Note on the dependency direction: `internal/mcp` must not import `internal/resources`. The server therefore holds a narrow interface it defines itself.

- [ ] **Step 1: Write the failing test**

Create `internal/resources/registry_test.go`:

```go
package resources

import (
	"context"
	"errors"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
)

// stubHandler serves one fixed URI.
type stubHandler struct {
	uri  string
	body string
}

func (s *stubHandler) Definitions() []mcp.Resource {
	return []mcp.Resource{{URI: s.uri, Name: "stub"}}
}

func (s *stubHandler) Templates() []mcp.ResourceTemplate { return nil }

func (s *stubHandler) Matches(uri string) bool { return uri == s.uri }

func (s *stubHandler) Read(ctx context.Context, uri string) (mcp.ResourceContents, error) {
	return mcp.ResourceContents{URI: uri, MIMEType: "text/plain", Text: s.body}, nil
}

func TestRegistryReadRoutesToHandler(t *testing.T) {
	r := NewResourceRegistry()
	if err := r.Register(&stubHandler{uri: "zosmf:///systems", body: "hello"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := r.Read(context.Background(), "zosmf:///systems")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Text != "hello" {
		t.Errorf("Text = %q, want hello", got.Text)
	}
}

func TestRegistryUnknownURI(t *testing.T) {
	r := NewResourceRegistry()

	_, err := r.Read(context.Background(), "zosmf://S0W1/dataset/USER.DATA")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRegistryRejectsDuplicateURI(t *testing.T) {
	r := NewResourceRegistry()
	if err := r.Register(&stubHandler{uri: "zosmf:///systems"}); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register(&stubHandler{uri: "zosmf:///systems"}); err == nil {
		t.Error("second Register succeeded, want error")
	}
}

func TestRegistryListAndTemplates(t *testing.T) {
	r := NewResourceRegistry()
	_ = r.Register(&stubHandler{uri: "zosmf:///systems"})

	if got := len(r.List()); got != 1 {
		t.Errorf("len(List()) = %d, want 1", got)
	}
	if got := len(r.Templates()); got != 0 {
		t.Errorf("len(Templates()) = %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/resources/ -run Registry -v`
Expected: FAIL — `NewResourceRegistry` and `ErrNotFound` undefined.

- [ ] **Step 3: Write the registry**

Create `internal/resources/registry.go`:

```go
package resources

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
)

// ErrNotFound is returned when no handler claims a URI. The server maps it
// to JSON-RPC -32002.
var ErrNotFound = errors.New("resource not found")

// ResourceHandler serves one family of resources.
type ResourceHandler interface {
	// Definitions returns concrete, listable resources. Most handlers
	// return nil: a system's datasets cannot be enumerated.
	Definitions() []mcp.Resource

	// Templates returns the URI templates this handler serves.
	Templates() []mcp.ResourceTemplate

	// Matches reports whether this handler serves the given URI.
	Matches(uri string) bool

	// Read fetches the content behind the URI.
	Read(ctx context.Context, uri string) (mcp.ResourceContents, error)
}

// ResourceRegistry routes resource URIs to handlers.
type ResourceRegistry struct {
	mu       sync.RWMutex
	handlers []ResourceHandler
	seen     map[string]bool // guards against two handlers claiming one URI
}

func NewResourceRegistry() *ResourceRegistry {
	return &ResourceRegistry{seen: make(map[string]bool)}
}

func (r *ResourceRegistry) Register(h ResourceHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, def := range h.Definitions() {
		if r.seen[def.URI] {
			return fmt.Errorf("resource already registered: %s", def.URI)
		}
		r.seen[def.URI] = true
	}
	r.handlers = append(r.handlers, h)
	return nil
}

func (r *ResourceRegistry) List() []mcp.Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []mcp.Resource
	for _, h := range r.handlers {
		out = append(out, h.Definitions()...)
	}
	return out
}

func (r *ResourceRegistry) Templates() []mcp.ResourceTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []mcp.ResourceTemplate
	for _, h := range r.handlers {
		out = append(out, h.Templates()...)
	}
	return out
}

func (r *ResourceRegistry) Read(ctx context.Context, uri string) (mcp.ResourceContents, error) {
	r.mu.RLock()
	handlers := make([]ResourceHandler, len(r.handlers))
	copy(handlers, r.handlers)
	r.mu.RUnlock()

	for _, h := range handlers {
		if h.Matches(uri) {
			return h.Read(ctx, uri)
		}
	}
	return mcp.ResourceContents{}, fmt.Errorf("%w: %s", ErrNotFound, uri)
}
```

- [ ] **Step 4: Run the registry tests**

Run: `go test ./internal/resources/ -run Registry -v`
Expected: PASS.

- [ ] **Step 5: Wire the dispatch into the server**

In `internal/mcp/server.go`, add the provider interface and field. Place the interface near the top of the file, after the imports:

```go
// ResourceProvider is the narrow view the server needs of a resource
// registry. Defined here so internal/mcp does not import the resources
// package, which would create a cycle.
type ResourceProvider interface {
	List() []Resource
	Templates() []ResourceTemplate
	Read(ctx context.Context, uri string) (ResourceContents, error)
}
```

Add the field to the `Server` struct next to `registry`:

```go
	resources ResourceProvider
```

Add the setter after `NewHTTPServer`:

```go
// SetResourceProvider attaches a resource registry. When nil, the three
// resources/* methods return method-not-found.
func (s *Server) SetResourceProvider(p ResourceProvider) {
	s.resources = p
}
```

Add the three branches to **both** switch statements — the `httpMode` one around line 107 and the ready-state one around line 136:

```go
		case "resources/list":
			return s.handleResourcesList(msg)
		case "resources/templates/list":
			return s.handleResourceTemplatesList(msg)
		case "resources/read":
			return s.handleResourcesRead(ctx, msg)
```

Add the handlers at the end of the file:

```go
func (s *Server) handleResourcesList(msg *JSONRPCMessage) *JSONRPCMessage {
	if s.resources == nil {
		return NewErrorResponse(msg.ID, ErrCodeMethodNotFound, "resources are not enabled")
	}
	resp, err := NewResponse(msg.ID, ResourcesListResult{Resources: s.resources.List()})
	if err != nil {
		return NewErrorResponse(msg.ID, ErrCodeInternal, err.Error())
	}
	return resp
}

func (s *Server) handleResourceTemplatesList(msg *JSONRPCMessage) *JSONRPCMessage {
	if s.resources == nil {
		return NewErrorResponse(msg.ID, ErrCodeMethodNotFound, "resources are not enabled")
	}
	resp, err := NewResponse(msg.ID, ResourceTemplatesListResult{
		ResourceTemplates: s.resources.Templates(),
	})
	if err != nil {
		return NewErrorResponse(msg.ID, ErrCodeInternal, err.Error())
	}
	return resp
}

func (s *Server) handleResourcesRead(ctx context.Context, msg *JSONRPCMessage) *JSONRPCMessage {
	if s.resources == nil {
		return NewErrorResponse(msg.ID, ErrCodeMethodNotFound, "resources are not enabled")
	}

	var params ResourcesReadParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return NewErrorResponse(msg.ID, ErrCodeInvalidParams, "invalid resources/read params")
	}
	if params.URI == "" {
		return NewErrorResponse(msg.ID, ErrCodeInvalidParams, "uri is required")
	}

	contents, err := s.resources.Read(ctx, params.URI)
	if err != nil {
		return NewErrorResponse(msg.ID, ErrCodeResourceNotFound, err.Error())
	}

	resp, err := NewResponse(msg.ID, ResourcesReadResult{Contents: []ResourceContents{contents}})
	if err != nil {
		return NewErrorResponse(msg.ID, ErrCodeInternal, err.Error())
	}
	return resp
}
```

- [ ] **Step 6: Run the full suite**

Run: `make test`
Expected: PASS, all packages.

- [ ] **Step 7: Commit**

```bash
git add internal/resources/ internal/mcp/
git commit -m "feat(resources): add registry and resources/* dispatch"
```

---

### Task 4: System inventory resource

**Files:**
- Create: `internal/resources/systems.go`
- Test: `internal/resources/systems_test.go`

**Interfaces:**
- Consumes: `resources.SystemsURI`, `zosmf.ClientRegistry.SystemNames()`, `config.Config`.
- Produces: `resources.NewSystemsHandler(clients *zosmf.ClientRegistry, cfg *config.Config) ResourceHandler`.

- [ ] **Step 1: Write the failing test**

Create `internal/resources/systems_test.go`:

```go
package resources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

func TestSystemsHandlerListsConfiguredSystems(t *testing.T) {
	cfg := &config.Config{
		Systems: []config.SystemConfig{
			{Name: "S0W1", Host: "https://zosmf.example.com:443", CSIDatasets: []string{"SYS1.SMPE.CSI"}},
			{Name: "S0W2", Host: "https://zosmf2.example.com:443"},
		},
	}
	clients := zosmf.NewClientRegistry()

	h := NewSystemsHandler(clients, cfg)

	defs := h.Definitions()
	if len(defs) != 1 {
		t.Fatalf("len(Definitions()) = %d, want 1", len(defs))
	}
	if defs[0].URI != SystemsURI {
		t.Errorf("URI = %q, want %q", defs[0].URI, SystemsURI)
	}

	if !h.Matches(SystemsURI) {
		t.Error("Matches(SystemsURI) = false, want true")
	}
	if h.Matches("zosmf://S0W1/dataset/USER.DATA") {
		t.Error("Matches claimed a dataset URI")
	}

	got, err := h.Read(context.Background(), SystemsURI)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.MIMEType != "application/json" {
		t.Errorf("MIMEType = %q, want application/json", got.MIMEType)
	}

	var payload struct {
		Systems []struct {
			Name        string   `json:"name"`
			Host        string   `json:"host"`
			CSIDatasets []string `json:"csi_datasets,omitempty"`
		} `json:"systems"`
	}
	if err := json.Unmarshal([]byte(got.Text), &payload); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if len(payload.Systems) != 2 {
		t.Fatalf("len(systems) = %d, want 2", len(payload.Systems))
	}
	if payload.Systems[0].Name != "S0W1" {
		t.Errorf("first system = %q, want S0W1", payload.Systems[0].Name)
	}
	if strings.Contains(got.Text, "password") {
		t.Error("payload leaks credential fields")
	}
}

func TestSystemsHandlerRejectsOtherURI(t *testing.T) {
	h := NewSystemsHandler(zosmf.NewClientRegistry(), &config.Config{})

	if _, err := h.Read(context.Background(), "zosmf://S0W1/dataset/USER.DATA"); err == nil {
		t.Error("Read of a foreign URI succeeded, want error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/resources/ -run Systems -v`
Expected: FAIL — `NewSystemsHandler` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/resources/systems.go`:

```go
package resources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

// systemsHandler serves the server-level inventory. It is the only listable
// resource: everything else on a z/OS system needs a name to address it.
type systemsHandler struct {
	clients *zosmf.ClientRegistry
	cfg     *config.Config
}

// systemEntry is the shape written into the inventory. Deliberately narrow —
// no auth block, so credentials cannot leak into a resource body.
type systemEntry struct {
	Name        string   `json:"name"`
	Host        string   `json:"host"`
	CSIDatasets []string `json:"csi_datasets,omitempty"`
	TargetZones []string `json:"target_zones,omitempty"`
}

func NewSystemsHandler(clients *zosmf.ClientRegistry, cfg *config.Config) ResourceHandler {
	return &systemsHandler{clients: clients, cfg: cfg}
}

func (h *systemsHandler) Definitions() []mcp.Resource {
	return []mcp.Resource{{
		URI:         SystemsURI,
		Name:        "Configured z/OS systems",
		Description: "The z/OSMF endpoints this server talks to, with their SMP/E CSI datasets and target zones.",
		MIMEType:    "application/json",
	}}
}

func (h *systemsHandler) Templates() []mcp.ResourceTemplate { return nil }

func (h *systemsHandler) Matches(uri string) bool { return uri == SystemsURI }

func (h *systemsHandler) Read(_ context.Context, uri string) (mcp.ResourceContents, error) {
	if uri != SystemsURI {
		return mcp.ResourceContents{}, fmt.Errorf("%w: %s", ErrNotFound, uri)
	}

	entries := make([]systemEntry, 0, len(h.cfg.Systems))
	for _, s := range h.cfg.Systems {
		entries = append(entries, systemEntry{
			Name:        s.Name,
			Host:        s.Host,
			CSIDatasets: s.CSIDatasets,
			TargetZones: s.TargetZones,
		})
	}

	body, err := json.MarshalIndent(map[string]any{"systems": entries}, "", "  ")
	if err != nil {
		return mcp.ResourceContents{}, fmt.Errorf("building system inventory: %w", err)
	}

	return mcp.ResourceContents{
		URI:      SystemsURI,
		MIMEType: "application/json",
		Text:     string(body),
	}, nil
}
```

- [ ] **Step 4: Run tests**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/resources/
git commit -m "feat(resources): add system inventory resource"
```

---

### Task 5: Content helpers — binary detection and size ceiling

**Files:**
- Create: `internal/resources/content.go`
- Test: `internal/resources/content_test.go`

**Interfaces:**
- Consumes: `mcp.ResourceContents`.
- Produces: `resources.MaxReadBytes` constant, `resources.buildContents(uri, mimeType, body string) (mcp.ResourceContents, error)`, `resources.ErrTooLarge`.

These are shared by tasks 6, 7 and 8, so they get their own task and their own tests.

- [ ] **Step 1: Write the failing test**

Create `internal/resources/content_test.go`:

```go
package resources

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestBuildContentsText(t *testing.T) {
	got, err := buildContents("zosmf://S0W1/dataset/USER.DATA", "text/plain", "hello\nworld\n")
	if err != nil {
		t.Fatalf("buildContents: %v", err)
	}
	if got.Text != "hello\nworld\n" {
		t.Errorf("Text = %q", got.Text)
	}
	if got.Blob != "" {
		t.Error("Blob set on text content")
	}
	if got.MIMEType != "text/plain" {
		t.Errorf("MIMEType = %q, want text/plain", got.MIMEType)
	}
}

func TestBuildContentsBinaryBecomesBlob(t *testing.T) {
	body := string([]byte{0x00, 0xff, 0xfe, 0x01})

	got, err := buildContents("zosmf://S0W1/dataset/SYS1.LINKLIB(IEFBR14)", "text/plain", body)
	if err != nil {
		t.Fatalf("buildContents: %v", err)
	}
	if got.Text != "" {
		t.Error("Text set on binary content")
	}
	if got.MIMEType != "application/octet-stream" {
		t.Errorf("MIMEType = %q, want application/octet-stream", got.MIMEType)
	}
	decoded, err := base64.StdEncoding.DecodeString(got.Blob)
	if err != nil {
		t.Fatalf("Blob is not valid base64: %v", err)
	}
	if string(decoded) != body {
		t.Error("Blob does not round-trip to the original bytes")
	}
}

func TestBuildContentsRejectsOversize(t *testing.T) {
	body := strings.Repeat("a", MaxReadBytes+1)

	_, err := buildContents("zosmf://S0W1/dataset/BIG.DATA", "text/plain", body)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	if !strings.Contains(err.Error(), "10485761") {
		t.Errorf("error should name the actual size, got: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/resources/ -run BuildContents -v`
Expected: FAIL — `buildContents`, `MaxReadBytes`, `ErrTooLarge` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/resources/content.go`:

```go
package resources

import (
	"encoding/base64"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
)

// MaxReadBytes caps a single resources/read. The client asked for this
// object deliberately, so there is no threshold — but an accidental read of a
// very large dataset must not take the process down. Not configurable.
const MaxReadBytes = 10 * 1024 * 1024

// ErrTooLarge is returned when a resource body exceeds MaxReadBytes.
var ErrTooLarge = errors.New("resource too large")

// buildContents packages a fetched body. Content that is not valid UTF-8 —
// load modules, zFS aggregates — goes out as a base64 blob rather than as
// broken text.
func buildContents(uri, mimeType, body string) (mcp.ResourceContents, error) {
	if len(body) > MaxReadBytes {
		return mcp.ResourceContents{}, fmt.Errorf("%w: %d bytes exceeds the %d byte limit",
			ErrTooLarge, len(body), MaxReadBytes)
	}

	if !utf8.ValidString(body) {
		return mcp.ResourceContents{
			URI:      uri,
			MIMEType: "application/octet-stream",
			Blob:     base64.StdEncoding.EncodeToString([]byte(body)),
		}, nil
	}

	return mcp.ResourceContents{URI: uri, MIMEType: mimeType, Text: body}, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/resources/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/resources/
git commit -m "feat(resources): add binary detection and read size ceiling"
```

---

### Task 6: Dataset and USS resources

**Files:**
- Create: `internal/resources/datasets.go`
- Create: `internal/resources/ussfiles.go`
- Test: `internal/resources/datasets_test.go`
- Test: `internal/resources/ussfiles_test.go`

**Interfaces:**
- Consumes: `resources.ParseURI`, `resources.buildContents`, `zosmf.Client.ReadDataset(dsname, encoding string) (string, error)`, `zosmf.Client.ReadUSSFile(path, tag string) (string, error)`, `zosmf.ClientRegistry.Get(name string) (*Client, error)`.
- Produces: `resources.NewDatasetHandler(clients *zosmf.ClientRegistry) ResourceHandler`, `resources.NewUSSHandler(clients *zosmf.ClientRegistry) ResourceHandler`.

Both handlers pass an empty `encoding`/`tag`, which selects the z/OSMF default conversion. A specific code page is what the tools are for.

- [ ] **Step 1: Write the failing dataset test**

Create `internal/resources/datasets_test.go`:

```go
package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

// newTestClients builds a ClientRegistry pointed at a test server.
// zosmf.NewClient takes a config.SystemConfig — see internal/zosmf/client.go:67.
func newTestClients(t *testing.T, srvURL string) *zosmf.ClientRegistry {
	t.Helper()

	reg := zosmf.NewClientRegistry()
	client, err := zosmf.NewClient(config.SystemConfig{Name: "S0W1", Host: srvURL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	reg.Register("S0W1", client)
	return reg
}

func TestDatasetResourceRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "SYS1.PARMLIB(IEASYS00)") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("SYSCAT=CATALOG\nSYSPLEX=PLEX1\n"))
	}))
	defer srv.Close()

	h := NewDatasetHandler(newTestClients(t, srv.URL))
	uri := DatasetURI("S0W1", "SYS1.PARMLIB(IEASYS00)")

	if !h.Matches(uri) {
		t.Fatal("Matches = false, want true")
	}

	got, err := h.Read(context.Background(), uri)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(got.Text, "SYSPLEX=PLEX1") {
		t.Errorf("Text = %q", got.Text)
	}
	if got.URI != uri {
		t.Errorf("URI = %q, want %q", got.URI, uri)
	}
}

func TestDatasetResourceNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := NewDatasetHandler(newTestClients(t, srv.URL))

	_, err := h.Read(context.Background(), DatasetURI("S0W1", "NO.SUCH.DATA"))
	if err == nil {
		t.Fatal("Read succeeded, want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should say not found, got: %v", err)
	}
}

func TestDatasetResourceAccessDenied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	h := NewDatasetHandler(newTestClients(t, srv.URL))

	_, err := h.Read(context.Background(), DatasetURI("S0W1", "SECRET.DATA"))
	if err == nil {
		t.Fatal("Read succeeded, want error")
	}
	if !strings.Contains(err.Error(), "SAF/RACF") {
		t.Errorf("error should name SAF/RACF, got: %v", err)
	}
	if strings.Contains(err.Error(), srv.URL) {
		t.Error("error leaks the request URL")
	}
}

func TestDatasetResourceUnknownSystem(t *testing.T) {
	h := NewDatasetHandler(zosmf.NewClientRegistry())

	if _, err := h.Read(context.Background(), DatasetURI("NOPE", "USER.DATA")); err == nil {
		t.Error("Read with unknown system succeeded, want error")
	}
}

func TestDatasetTemplates(t *testing.T) {
	h := NewDatasetHandler(zosmf.NewClientRegistry())

	tmpl := h.Templates()
	if len(tmpl) != 1 {
		t.Fatalf("len(Templates()) = %d, want 1", len(tmpl))
	}
	if tmpl[0].URITemplate != "zosmf://{system}/dataset/{dsn}" {
		t.Errorf("URITemplate = %q", tmpl[0].URITemplate)
	}
	if h.Definitions() != nil {
		t.Error("Definitions() should be nil — datasets cannot be enumerated")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/resources/ -run Dataset -v`
Expected: FAIL — `NewDatasetHandler` undefined.

- [ ] **Step 3: Write the dataset handler**

Create `internal/resources/datasets.go`:

```go
package resources

import (
	"context"
	"fmt"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

type datasetHandler struct {
	clients *zosmf.ClientRegistry
}

func NewDatasetHandler(clients *zosmf.ClientRegistry) ResourceHandler {
	return &datasetHandler{clients: clients}
}

func (h *datasetHandler) Definitions() []mcp.Resource { return nil }

func (h *datasetHandler) Templates() []mcp.ResourceTemplate {
	return []mcp.ResourceTemplate{{
		URITemplate: "zosmf://{system}/dataset/{dsn}",
		Name:        "Dataset or PDS member",
		Description: "A sequential dataset or a PDS member, for example zosmf://S0W1/dataset/SYS1.PARMLIB(IEASYS00).",
		MIMEType:    "text/plain",
	}}
}

func (h *datasetHandler) Matches(uri string) bool {
	ref, err := ParseURI(uri)
	return err == nil && ref.Kind == KindDataset
}

func (h *datasetHandler) Read(_ context.Context, uri string) (mcp.ResourceContents, error) {
	ref, err := ParseURI(uri)
	if err != nil {
		return mcp.ResourceContents{}, err
	}

	client, err := h.clients.Get(ref.System)
	if err != nil {
		return mcp.ResourceContents{}, err
	}

	body, err := client.ReadDataset(ref.Dataset, "")
	if err != nil {
		return mcp.ResourceContents{}, describeZOSMFError(err, "dataset "+ref.Dataset)
	}

	return buildContents(uri, "text/plain", body)
}
```

Add the shared error mapper to `internal/resources/content.go`:

```go
// describeZOSMFError reduces a z/OSMF failure to its statement. Response
// headers and request URLs are never passed through — they can carry
// credentials or internal hostnames.
func describeZOSMFError(err error, subject string) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "404"):
		return fmt.Errorf("%s not found", subject)
	case strings.Contains(msg, "403"):
		return fmt.Errorf("access to %s denied by SAF/RACF", subject)
	case strings.Contains(msg, "401"):
		return fmt.Errorf("authentication failed for %s", subject)
	}
	return fmt.Errorf("reading %s failed", subject)
}
```

Add `"strings"` to that file's imports.

- [ ] **Step 4: Run the dataset tests**

Run: `go test ./internal/resources/ -run Dataset -v`
Expected: PASS.

- [ ] **Step 5: Write the failing USS test**

Create `internal/resources/ussfiles_test.go`:

```go
package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

func TestUSSResourceRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/etc/profile") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("export PATH=/bin\n"))
	}))
	defer srv.Close()

	h := NewUSSHandler(newTestClients(t, srv.URL))
	uri := USSURI("S0W1", "/etc/profile")

	if !h.Matches(uri) {
		t.Fatal("Matches = false, want true")
	}

	got, err := h.Read(context.Background(), uri)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(got.Text, "export PATH=/bin") {
		t.Errorf("Text = %q", got.Text)
	}
}

func TestUSSResourceKeepsPathCase(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	h := NewUSSHandler(newTestClients(t, srv.URL))

	if _, err := h.Read(context.Background(), USSURI("S0W1", "/u/ibmuser/MyScript.sh")); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(seen, "MyScript.sh") {
		t.Errorf("USS path was case-folded: %s", seen)
	}
}

func TestUSSTemplates(t *testing.T) {
	h := NewUSSHandler(zosmf.NewClientRegistry())

	tmpl := h.Templates()
	if len(tmpl) != 1 {
		t.Fatalf("len(Templates()) = %d, want 1", len(tmpl))
	}
	if tmpl[0].URITemplate != "zosmf://{system}/uss/{path}" {
		t.Errorf("URITemplate = %q", tmpl[0].URITemplate)
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./internal/resources/ -run USS -v`
Expected: FAIL — `NewUSSHandler` undefined.

- [ ] **Step 7: Write the USS handler**

Create `internal/resources/ussfiles.go`:

```go
package resources

import (
	"context"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

type ussHandler struct {
	clients *zosmf.ClientRegistry
}

func NewUSSHandler(clients *zosmf.ClientRegistry) ResourceHandler {
	return &ussHandler{clients: clients}
}

func (h *ussHandler) Definitions() []mcp.Resource { return nil }

func (h *ussHandler) Templates() []mcp.ResourceTemplate {
	return []mcp.ResourceTemplate{{
		URITemplate: "zosmf://{system}/uss/{path}",
		Name:        "USS file",
		Description: "A z/OS UNIX file, for example zosmf://S0W1/uss/etc/profile. The path is case-sensitive.",
		MIMEType:    "text/plain",
	}}
}

func (h *ussHandler) Matches(uri string) bool {
	ref, err := ParseURI(uri)
	return err == nil && ref.Kind == KindUSS
}

func (h *ussHandler) Read(_ context.Context, uri string) (mcp.ResourceContents, error) {
	ref, err := ParseURI(uri)
	if err != nil {
		return mcp.ResourceContents{}, err
	}

	client, err := h.clients.Get(ref.System)
	if err != nil {
		return mcp.ResourceContents{}, err
	}

	body, err := client.ReadUSSFile(ref.Path, "")
	if err != nil {
		return mcp.ResourceContents{}, describeZOSMFError(err, "USS path "+ref.Path)
	}

	return buildContents(uri, "text/plain", body)
}
```

- [ ] **Step 8: Run the full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/resources/
git commit -m "feat(resources): add dataset and USS file resources"
```

---

### Task 7: Job spool resource

**Files:**
- Create: `internal/resources/jobspool.go`
- Test: `internal/resources/jobspool_test.go`

**Interfaces:**
- Consumes: `resources.ParseURI`, `resources.buildContents`, `zosmf.Client.GetJobFileContent(jobName, jobID string, fileID int) (string, error)`.
- Produces: `resources.NewJobSpoolHandler(clients *zosmf.ClientRegistry) ResourceHandler`.

- [ ] **Step 1: Write the failing test**

Create `internal/resources/jobspool_test.go`:

```go
package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

func TestJobSpoolResourceRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/files/2/records") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("IEF142I MYJOB GO - STEP WAS EXECUTED\n"))
	}))
	defer srv.Close()

	h := NewJobSpoolHandler(newTestClients(t, srv.URL))
	uri := SpoolURI("S0W1", "MYJOB", "JOB12345", 2)

	if !h.Matches(uri) {
		t.Fatal("Matches = false, want true")
	}

	got, err := h.Read(context.Background(), uri)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(got.Text, "IEF142I") {
		t.Errorf("Text = %q", got.Text)
	}
}

func TestJobSpoolResourceNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := NewJobSpoolHandler(newTestClients(t, srv.URL))

	_, err := h.Read(context.Background(), SpoolURI("S0W1", "MYJOB", "JOB99999", 1))
	if err == nil {
		t.Fatal("Read succeeded, want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should say not found, got: %v", err)
	}
}

func TestJobSpoolTemplates(t *testing.T) {
	h := NewJobSpoolHandler(zosmf.NewClientRegistry())

	tmpl := h.Templates()
	if len(tmpl) != 1 {
		t.Fatalf("len(Templates()) = %d, want 1", len(tmpl))
	}
	if tmpl[0].URITemplate != "zosmf://{system}/job/{jobname}/{jobid}/spool/{id}" {
		t.Errorf("URITemplate = %q", tmpl[0].URITemplate)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/resources/ -run JobSpool -v`
Expected: FAIL — `NewJobSpoolHandler` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/resources/jobspool.go`:

```go
package resources

import (
	"context"
	"fmt"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

type jobSpoolHandler struct {
	clients *zosmf.ClientRegistry
}

func NewJobSpoolHandler(clients *zosmf.ClientRegistry) ResourceHandler {
	return &jobSpoolHandler{clients: clients}
}

func (h *jobSpoolHandler) Definitions() []mcp.Resource { return nil }

func (h *jobSpoolHandler) Templates() []mcp.ResourceTemplate {
	return []mcp.ResourceTemplate{{
		URITemplate: "zosmf://{system}/job/{jobname}/{jobid}/spool/{id}",
		Name:        "Job spool file",
		Description: "One spool file of a job, addressed by the numeric id z/OSMF assigns. Several steps can share a DD name, so the id rather than the DD name identifies the file.",
		MIMEType:    "text/plain",
	}}
}

func (h *jobSpoolHandler) Matches(uri string) bool {
	ref, err := ParseURI(uri)
	return err == nil && ref.Kind == KindSpool
}

func (h *jobSpoolHandler) Read(_ context.Context, uri string) (mcp.ResourceContents, error) {
	ref, err := ParseURI(uri)
	if err != nil {
		return mcp.ResourceContents{}, err
	}

	client, err := h.clients.Get(ref.System)
	if err != nil {
		return mcp.ResourceContents{}, err
	}

	body, err := client.GetJobFileContent(ref.JobName, ref.JobID, ref.FileID)
	if err != nil {
		subject := fmt.Sprintf("spool file %d of job %s/%s", ref.FileID, ref.JobName, ref.JobID)
		return mcp.ResourceContents{}, describeZOSMFError(err, subject)
	}

	return buildContents(uri, "text/plain", body)
}
```

- [ ] **Step 4: Run the full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/resources/
git commit -m "feat(resources): add job spool file resource"
```

---

### Task 8: Threshold helpers and single-file read tools

**Files:**
- Create: `internal/tools/largecontent.go`
- Test: `internal/tools/largecontent_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/tools/datasets.go` (`readDatasetTool.Execute`)
- Modify: `internal/tools/ussfiles.go` (`readUSSFileTool.Execute`)
- Modify: `internal/tools/jobs.go` (`getSpoolContentTool.Execute`)
- Modify: `internal/tools/registry.go` (`RegisterAll` signature)

**Interfaces:**
- Consumes: `resources.DatasetURI`, `resources.USSURI`, `resources.SpoolURI`, `mcp.ResourceLink`, `mcp.TextContent`.
- Produces: `tools.DefaultInlineMaxBytes` constant, `tools.previewBytes` constant, `tools.preview(text string) string`, `tools.textOrLink(text string, max int, link mcp.Content) []mcp.Content`, `config.ResourcesConfig` with field `InlineMaxBytes int`. `RegisterAll` gains no new parameter — the threshold is read from `cfg`, which it already receives.

- [ ] **Step 1: Write the failing test**

Create `internal/tools/largecontent_test.go`:

```go
package tools

import (
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
)

func TestTextOrLinkBelowThreshold(t *testing.T) {
	link := mcp.ResourceLink("zosmf://S0W1/dataset/USER.DATA", "USER.DATA", "", "text/plain", 5)

	got := textOrLink("hello", 100, link)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Type != "text" {
		t.Errorf("Type = %q, want text", got[0].Type)
	}
	if got[0].Text != "hello" {
		t.Errorf("Text = %q, want hello", got[0].Text)
	}
}

func TestTextOrLinkAboveThreshold(t *testing.T) {
	body := strings.Repeat("line of output\n", 1000) // 15000 bytes
	link := mcp.ResourceLink("zosmf://S0W1/dataset/BIG.DATA", "BIG.DATA", "", "text/plain", int64(len(body)))

	got := textOrLink(body, 1000, link)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (preview + link)", len(got))
	}
	if got[0].Type != "text" {
		t.Errorf("first Type = %q, want text", got[0].Type)
	}
	if len(got[0].Text) > previewBytes {
		t.Errorf("preview is %d bytes, want at most %d", len(got[0].Text), previewBytes)
	}
	if got[1].Type != "resource_link" {
		t.Errorf("second Type = %q, want resource_link", got[1].Type)
	}
	if got[1].Size != int64(len(body)) {
		t.Errorf("Size = %d, want %d", got[1].Size, len(body))
	}
}

func TestPreviewCutsAtLineBoundary(t *testing.T) {
	body := strings.Repeat("0123456789\n", 500) // 5500 bytes, 11-byte lines

	got := preview(body)

	if len(got) > previewBytes {
		t.Fatalf("preview is %d bytes, want at most %d", len(got), previewBytes)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("preview does not end at a line boundary: %q", got[len(got)-20:])
	}
}

func TestPreviewOfSingleLongLine(t *testing.T) {
	// No newline to cut at — must still respect the byte cap.
	body := strings.Repeat("x", previewBytes*2)

	got := preview(body)

	if len(got) > previewBytes {
		t.Errorf("preview is %d bytes, want at most %d", len(got), previewBytes)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tools/ -run 'TextOrLink|Preview' -v`
Expected: FAIL — `textOrLink`, `preview`, `previewBytes` undefined.

- [ ] **Step 3: Write the helpers**

Create `internal/tools/largecontent.go`:

```go
package tools

import (
	"strings"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
)

// DefaultInlineMaxBytes is how much content a tool result may carry as inline
// text before it switches to a preview plus a resource_link. Roughly eight to
// ten thousand tokens — enough for JESYSMSG and a typical PARMLIB member, too
// little for the SYSPRINT of a compile.
const DefaultInlineMaxBytes = 32768

// previewBytes is how much of an oversized body still goes inline. A
// constant, not a configuration value: a second knob for something nobody
// wants to tune.
const previewBytes = 2048

// preview returns the head of text, cut at the last line boundary that fits
// so the result does not end mid-record.
func preview(text string) string {
	if len(text) <= previewBytes {
		return text
	}
	head := text[:previewBytes]
	if i := strings.LastIndexByte(head, '\n'); i > 0 {
		return head[:i+1]
	}
	return head
}

// textOrLink returns the body as inline text when it fits within max, and a
// preview plus the given resource_link when it does not. The preview goes out
// even to clients that support resources: it costs little and means a result
// is never a bare pointer.
func textOrLink(text string, max int, link mcp.Content) []mcp.Content {
	if len(text) <= max {
		return []mcp.Content{mcp.TextContent(text)}
	}
	return []mcp.Content{mcp.TextContent(preview(text)), link}
}
```

- [ ] **Step 4: Run the helper tests**

Run: `go test ./internal/tools/ -run 'TextOrLink|Preview' -v`
Expected: PASS.

- [ ] **Step 5: Add the configuration field**

In `internal/config/config.go`, add to the `Config` struct:

```go
	Resources    ResourcesConfig    `yaml:"resources"`
```

Add the type next to the other config types:

```go
// ResourcesConfig tunes how much content a tool result carries inline before
// it switches to a resource_link.
type ResourcesConfig struct {
	InlineMaxBytes int `yaml:"inline_max_bytes"`
}
```

Apply the default in the existing normalization function — find it with `grep -n "func (c \*Config)" internal/config/config.go` and add:

```go
	if c.Resources.InlineMaxBytes <= 0 {
		c.Resources.InlineMaxBytes = defaultInlineMaxBytes
	}
```

with the constant declared in `internal/config/config.go`:

```go
// defaultInlineMaxBytes mirrors tools.DefaultInlineMaxBytes. Duplicated
// rather than imported: internal/config must not depend on internal/tools.
const defaultInlineMaxBytes = 32768
```

- [ ] **Step 6: Write the failing tool test**

Add to `internal/tools/largecontent_test.go`:

```go
func TestReadDatasetReturnsLinkWhenLarge(t *testing.T) {
	body := strings.Repeat("record of data\n", 4000) // 60000 bytes

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	tool := NewReadDatasetTool(newToolTestClients(t, srv.URL))
	args, _ := json.Marshal(map[string]any{"system": "S0W1", "dataset": "BIG.DATA"})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Content) != 2 {
		t.Fatalf("len(Content) = %d, want 2", len(result.Content))
	}
	if result.Content[1].Type != "resource_link" {
		t.Errorf("second content Type = %q, want resource_link", result.Content[1].Type)
	}
	if result.Content[1].URI != "zosmf://S0W1/dataset/BIG.DATA" {
		t.Errorf("URI = %q", result.Content[1].URI)
	}
}
```

Add a `newToolTestClients` helper to the same file, mirroring `newTestClients` from Task 6 but in package `tools`. Add the needed imports: `context`, `encoding/json`, `net/http`, `net/http/httptest`, and `internal/zosmf`.

If `NewReadDatasetTool` currently takes only `clients`, it now also needs the threshold. Change its constructor to `NewReadDatasetTool(clients *zosmf.ClientRegistry, inlineMax int)` and pass `cfg.Resources.InlineMaxBytes` from `RegisterAll`. Update the test call accordingly.

- [ ] **Step 7: Run test to verify it fails**

Run: `go test ./internal/tools/ -run ReadDatasetReturnsLink -v`
Expected: FAIL — one content item returned, not two.

- [ ] **Step 8: Apply the threshold in the three single-file tools**

In `internal/tools/datasets.go`, in `readDatasetTool`, add an `inlineMax int` field and replace the final `return mcp.TextResult(content), nil` with:

```go
	link := mcp.ResourceLink(
		resources.DatasetURI(args.System, args.Dataset),
		args.Dataset,
		"full dataset content",
		"text/plain",
		int64(len(content)),
	)
	return mcp.ToolCallResult{Content: textOrLink(content, t.inlineMax, link)}, nil
```

In `internal/tools/ussfiles.go`, in `readUSSFileTool`, the same shape with `resources.USSURI(args.System, args.Path)` and the path as the link name.

In `internal/tools/jobs.go`, in `getSpoolContentTool`, the same shape with `resources.SpoolURI(args.System, args.JobName, args.JobID, args.FileID)`.

Add the import `"github.com/cybersorcerer/zosmf-mcp/internal/resources"` to each of those three files, and thread `cfg.Resources.InlineMaxBytes` through `RegisterAll` into the three constructors.

- [ ] **Step 9: Run the full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/tools/ internal/config/
git commit -m "feat(tools): return resource_link for oversized read results"
```

---

### Task 9: Spool budget in get_all_spools

**Files:**
- Modify: `internal/tools/jobs.go:285-310` (`getAllSpoolsTool.Execute`)
- Test: `internal/tools/jobs_spool_budget_test.go` (create)

**Interfaces:**
- Consumes: `tools.preview`, `resources.SpoolURI`, `mcp.ResourceLink`, `zosmf.JobFile` (fields `ID int`, `DDName string`, `StepName string`, `ByteCount int64`).
- Produces: nothing new.

This task carries the binding success criterion from the design doc.

- [ ] **Step 1: Write the failing test**

Create `internal/tools/jobs_spool_budget_test.go`:

```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The success criterion from docs/2026-08-17-mcp-resources-design.md:
// small JES files inline, the large SYSPRINT as a link.
func TestGetAllSpoolsBudget(t *testing.T) {
	jesysmsg := strings.Repeat("J", 2*1024)
	jesjcl := strings.Repeat("C", 4*1024)
	sysprint := strings.Repeat("P", 200*1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/files"):
			_, _ = fmt.Fprintf(w, `[
				{"id":1,"ddname":"JESYSMSG","stepname":"JES2","byte-count":%d},
				{"id":2,"ddname":"JESJCL","stepname":"JES2","byte-count":%d},
				{"id":3,"ddname":"SYSPRINT","stepname":"GO","byte-count":%d}
			]`, len(jesysmsg), len(jesjcl), len(sysprint))
		case strings.HasSuffix(r.URL.Path, "/files/1/records"):
			_, _ = w.Write([]byte(jesysmsg))
		case strings.HasSuffix(r.URL.Path, "/files/2/records"):
			_, _ = w.Write([]byte(jesjcl))
		case strings.HasSuffix(r.URL.Path, "/files/3/records"):
			_, _ = w.Write([]byte(sysprint))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	tool := NewGetAllSpoolsTool(newToolTestClients(t, srv.URL), DefaultInlineMaxBytes)
	args, _ := json.Marshal(map[string]any{
		"system": "S0W1", "job_name": "MYJOB", "job_id": "JOB12345",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var inlineText string
	var links []string
	for _, c := range result.Content {
		switch c.Type {
		case "text":
			inlineText += c.Text
		case "resource_link":
			links = append(links, c.URI)
		}
	}

	if !strings.Contains(inlineText, jesysmsg) {
		t.Error("JESYSMSG is not inline, want full content")
	}
	if !strings.Contains(inlineText, jesjcl) {
		t.Error("JESJCL is not inline, want full content")
	}
	if strings.Contains(inlineText, sysprint) {
		t.Error("SYSPRINT is inline, want a link")
	}

	if len(links) != 1 {
		t.Fatalf("len(links) = %d, want 1", len(links))
	}
	if links[0] != "zosmf://S0W1/job/MYJOB/JOB12345/spool/3" {
		t.Errorf("link = %q", links[0])
	}
}

func TestGetAllSpoolsFirstFileOversized(t *testing.T) {
	huge := strings.Repeat("P", 200*1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/files"):
			_, _ = fmt.Fprintf(w, `[{"id":1,"ddname":"SYSPRINT","stepname":"GO","byte-count":%d}]`, len(huge))
		case strings.HasSuffix(r.URL.Path, "/files/1/records"):
			_, _ = w.Write([]byte(huge))
		}
	}))
	defer srv.Close()

	tool := NewGetAllSpoolsTool(newToolTestClients(t, srv.URL), DefaultInlineMaxBytes)
	args, _ := json.Marshal(map[string]any{
		"system": "S0W1", "job_name": "MYJOB", "job_id": "JOB12345",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var sawText, sawLink bool
	for _, c := range result.Content {
		if c.Type == "text" && len(c.Text) > 0 {
			sawText = true
		}
		if c.Type == "resource_link" {
			sawLink = true
		}
	}
	if !sawText {
		t.Error("no readable text — a result must never be link-only")
	}
	if !sawLink {
		t.Error("no link for the oversized file")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tools/ -run GetAllSpools -v`
Expected: FAIL — the tool concatenates everything, so SYSPRINT appears inline and no link is produced.

- [ ] **Step 3: Rewrite Execute**

Replace the body of `getAllSpoolsTool.Execute` in `internal/tools/jobs.go` after the `GetJobFiles` call:

```go
	var contents []mcp.Content
	budget := t.inlineMax

	for _, f := range files {
		uri := resources.SpoolURI(args.System, args.JobName, args.JobID, f.ID)
		desc := fmt.Sprintf("%s (step: %s)", f.DDName, f.StepName)

		content, err := client.GetJobFileContent(args.JobName, args.JobID, f.ID)
		if err != nil {
			contents = append(contents, mcp.TextContent(
				fmt.Sprintf("--- %s ---\n(error: %s)\n\n", desc, err.Error())))
			continue
		}

		link := mcp.ResourceLink(uri, f.DDName, desc, "text/plain", int64(len(content)))

		switch {
		case len(content) <= budget:
			contents = append(contents, mcp.TextContent(
				fmt.Sprintf("--- %s ---\n%s\n\n", desc, content)))
			budget -= len(content)

		case len(contents) == 0:
			// Nothing inline yet — emit a preview so the result is never a
			// bare pointer, then the link.
			contents = append(contents,
				mcp.TextContent(fmt.Sprintf("--- %s (truncated) ---\n%s\n\n", desc, preview(content))),
				link)
			budget = 0

		default:
			contents = append(contents, link)
		}
	}

	return mcp.ToolCallResult{Content: contents}, nil
```

Add the `inlineMax int` field to `getAllSpoolsTool`, extend `NewGetAllSpoolsTool` to `NewGetAllSpoolsTool(clients *zosmf.ClientRegistry, inlineMax int)`, and pass `cfg.Resources.InlineMaxBytes` from `RegisterAll`.

Update the tool description to match the new behaviour:

```go
		Description: "Read the spool files of a z/OS job. Small files are returned in full; files that would not fit are returned as resource links which can be read individually.",
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/tools/ -run GetAllSpools -v`
Expected: PASS.

- [ ] **Step 5: Fix the pre-existing test**

The old test for `zosmf_get_all_spools` expects full concatenation. Find it:

Run: `grep -rn "GetAllSpools" internal/tools/*_test.go`

Update its expectations to the new behaviour, or delete it if the two new tests fully supersede it.

- [ ] **Step 6: Run the full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/tools/
git commit -m "feat(tools): budget spool output, link files that do not fit"
```

---

### Task 10: Wiring, configuration example and documentation

**Files:**
- Modify: `cmd/zosmf-mcp/main.go`
- Modify: `internal/resources/registry.go` (add `RegisterAll`)
- Modify: `.zosmf-mcp-config.example.yaml`
- Modify: `README.md`
- Test: `internal/resources/registry_test.go` (extend)

**Interfaces:**
- Consumes: every handler constructor from tasks 4, 6 and 7.
- Produces: `resources.RegisterAll(registry *ResourceRegistry, clients *zosmf.ClientRegistry, cfg *config.Config) error`.

- [ ] **Step 1: Write the failing test**

Add to `internal/resources/registry_test.go`:

```go
func TestRegisterAllWiresEveryHandler(t *testing.T) {
	r := NewResourceRegistry()
	cfg := &config.Config{
		Systems: []config.SystemConfig{{Name: "S0W1", Host: "https://zosmf.example.com:443"}},
	}

	if err := RegisterAll(r, zosmf.NewClientRegistry(), cfg); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	if got := len(r.List()); got != 1 {
		t.Errorf("len(List()) = %d, want 1 (the system inventory)", got)
	}
	if got := len(r.Templates()); got != 3 {
		t.Errorf("len(Templates()) = %d, want 3 (dataset, uss, spool)", got)
	}
}
```

Add the imports `"github.com/cybersorcerer/zosmf-mcp/internal/config"` and `"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"` to that test file.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/resources/ -run RegisterAll -v`
Expected: FAIL — `RegisterAll` undefined.

- [ ] **Step 3: Write RegisterAll**

Append to `internal/resources/registry.go`:

```go
// RegisterAll registers every resource handler with the given registry.
func RegisterAll(registry *ResourceRegistry, clients *zosmf.ClientRegistry, cfg *config.Config) error {
	handlers := []ResourceHandler{
		NewSystemsHandler(clients, cfg),
		NewDatasetHandler(clients),
		NewUSSHandler(clients),
		NewJobSpoolHandler(clients),
	}

	for _, h := range handlers {
		if err := registry.Register(h); err != nil {
			return err
		}
	}
	return nil
}
```

Add the imports `"github.com/cybersorcerer/zosmf-mcp/internal/config"` and `"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"`.

- [ ] **Step 4: Run the test**

Run: `go test ./internal/resources/ -run RegisterAll -v`
Expected: PASS.

- [ ] **Step 5: Wire it in main**

In `cmd/zosmf-mcp/main.go`, after the tool registry is built and `tools.RegisterAll` has been called, add:

```go
	resourceRegistry := resources.NewResourceRegistry()
	if err := resources.RegisterAll(resourceRegistry, clients, cfg); err != nil {
		logger.Fatalf("registering resources: %v", err)
	}
```

Then attach it to every server instance that is created — search for `mcp.NewServer(` and `mcp.NewHTTPServer(` and add after each:

```go
	srv.SetResourceProvider(resourceRegistry)
```

Add the import `"github.com/cybersorcerer/zosmf-mcp/internal/resources"`.

- [ ] **Step 6: Verify the binary starts and lists resources**

Run: `make build`
Expected: builds clean.

Run:
```bash
printf '%s\n%s\n%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"resources/templates/list"}' \
  | ./bin/zosmf-mcp --transport stdio --config .zosmf-mcp-config.example.yaml
```
Expected: the initialize result advertises `"resources":{}` and the templates list contains three entries.

- [ ] **Step 7: Document the configuration**

In `.zosmf-mcp-config.example.yaml`, add a top-level block with a comment:

```yaml
# ---------------------------------------------------------------------------
# Resources
# How much content a tool result may carry as inline text before it switches
# to a preview plus a resource link. Clients read the full content through
# resources/read.
# ---------------------------------------------------------------------------
resources:
  inline_max_bytes: 32768
```

- [ ] **Step 8: Document the feature**

In `README.md`, add a section after the tools overview:

```markdown
## Resources

Besides tools, the server exposes z/OS objects as MCP resources. A client can
attach them as context directly, and tools return links to them instead of
dumping large content into the conversation.

| URI | What it addresses |
| --- | --- |
| `zosmf:///systems` | The configured z/OSMF endpoints |
| `zosmf://{system}/dataset/{dsn}` | A dataset or PDS member |
| `zosmf://{system}/uss/{path}` | A z/OS UNIX file |
| `zosmf://{system}/job/{jobname}/{jobid}/spool/{id}` | One spool file of a job |

Only `zosmf:///systems` appears in `resources/list` — the others are templates,
since a system's datasets cannot be enumerated.

Read results above `resources.inline_max_bytes` (32 KB by default) come back as
a preview plus a `resource_link`. `zosmf_get_all_spools` fills its budget with
the small JES files first and links the rest, so a job with a large SYSPRINT
still returns the messages that matter for diagnosis.

Resource access is bounded by SAF/RACF only, exactly like the read tools. See
[Before You Start](#️-before-you-start).
```

- [ ] **Step 9: Run everything**

Run: `make all`
Expected: fmt, vet, check-identifiers, test, build — all clean.

- [ ] **Step 10: Commit**

```bash
git add cmd/ internal/ README.md .zosmf-mcp-config.example.yaml
git commit -m "feat(resources): wire resources into the server, document them"
```

---

## Verification

After Task 10, the design's binding success criterion is covered by `TestGetAllSpoolsBudget` in `internal/tools/jobs_spool_budget_test.go` and the protocol compatibility criterion by `TestInitializeAcceptsOlderProtocolVersion` in `internal/mcp/resources_protocol_test.go`. Both run under `make test` and in CI.

Optional, not a gate: point a real client at the server and read a dataset resource; submit a job producing a large SYSPRINT with `IEBGENER` and confirm the link round-trips.
