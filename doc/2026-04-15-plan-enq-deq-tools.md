# ENQ/DEQ Dataset Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two new MCP tools to `zosmf_mcp` — `zosmf_acquire_dataset_enq` and `zosmf_release_dataset_enq` — that allow callers to acquire and release z/OS dataset ENQ locks independently of reading or writing content.

**Architecture:** Two new `ToolHandler` implementations in `internal/tools/datasets.go` (following the exact same pattern as existing tools), two new methods on `zosmf.Client` in `internal/zosmf/datasets.go`, and registration in `internal/tools/registry.go`. No new files needed.

**Tech Stack:** Go 1.22+, z/OSMF REST Files API (`/zosmf/restfiles/ds/`), existing `zosmf.Client` HTTP infrastructure.

**Design reference:** `doc/2026-04-15-enq-deq-tools-design.md`

**Prerequisite:** Understand the existing tool pattern by reading one example:
- `internal/tools/datasets.go` — existing `readDatasetTool` as structural reference
- `internal/zosmf/datasets.go` — existing `ReadDataset` method, `encodeDatasetPath` helper
- `internal/zosmf/client.go` — `Client.Do()` method, how custom headers are set
- `internal/mcp/protocol.go` — `ToolCallResult`, `TextResult()`, `ErrorResult()`

---

## File Map

| File | Action | What changes |
|------|--------|-------------|
| `internal/zosmf/datasets.go` | Modify | Add `AcquireDatasetEnq(dsname, enqType string) (string, error)` and `ReleaseDatasetEnq(dsname, sessionRef string) error` |
| `internal/tools/datasets.go` | Modify | Add `acquireDatasetEnqTool` and `releaseDatasetEnqTool` structs with `Definition()` and `Execute()` |
| `internal/tools/registry.go` | Modify | Register `NewAcquireDatasetEnqTool` and `NewReleaseDatasetEnqTool` in `RegisterAll` |

---

## Task 1: Add `AcquireDatasetEnq` and `ReleaseDatasetEnq` to the zosmf Client

**Files:**
- Modify: `internal/zosmf/datasets.go`

- [ ] **Step 1: Add `AcquireDatasetEnq` method at the end of `internal/zosmf/datasets.go`**

This method sends a GET to z/OSMF with the ENQ header. It reads the response body (z/OSMF requires a full GET to acquire ENQ) but discards it — the caller reads content separately. It returns the `X-IBM-Session-Ref` header value.

```go
// AcquireDatasetEnq acquires an ENQ lock on a dataset or PDS member without
// returning content. enqType must be "exclusive" (SHRW) or "shared" (SHRE).
// Returns the session reference string needed for subsequent operations and release.
func (c *Client) AcquireDatasetEnq(dsname, enqType string) (string, error) {
    var enqHeader string
    switch enqType {
    case "exclusive":
        enqHeader = "SHRW"
    case "shared":
        enqHeader = "SHRE"
    default:
        return "", fmt.Errorf("invalid enq_type: must be 'exclusive' or 'shared'")
    }

    path := fmt.Sprintf("/zosmf/restfiles/ds/%s", encodeDatasetPath(dsname))
    reqURL := c.baseURL + path
    req, err := http.NewRequest(http.MethodGet, reqURL, nil)
    if err != nil {
        return "", err
    }
    req.Header.Set("X-IBM-Data-Type", "text")
    req.Header.Set("X-IBM-Obtain-ENQ", enqHeader)

    resp, err := c.Do(req)
    if err != nil {
        return "", fmt.Errorf("acquire ENQ failed: %w", err)
    }
    defer resp.Body.Close()
    io.ReadAll(resp.Body) // discard body — content is read separately

    if resp.StatusCode == http.StatusConflict {
        return "", fmt.Errorf("ENQ_NOT_AVAILABLE: %s is locked by another user or session", dsname)
    }
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return "", fmt.Errorf("z/OSMF returned %d acquiring ENQ for %s", resp.StatusCode, dsname)
    }

    sessionRef := resp.Header.Get("X-IBM-Session-Ref")
    return sessionRef, nil
}
```

- [ ] **Step 2: Add `ReleaseDatasetEnq` method directly after `AcquireDatasetEnq`**

```go
// ReleaseDatasetEnq releases a previously acquired ENQ lock identified by sessionRef.
// Errors are non-fatal — z/OS will time out the ENQ automatically if release fails.
func (c *Client) ReleaseDatasetEnq(dsname, sessionRef string) error {
    path := fmt.Sprintf("/zosmf/restfiles/ds/%s", encodeDatasetPath(dsname))
    reqURL := c.baseURL + path
    req, err := http.NewRequest(http.MethodDelete, reqURL, nil)
    if err != nil {
        return err
    }
    req.Header.Set("X-IBM-Session-Ref", sessionRef)

    resp, err := c.Do(req)
    if err != nil {
        return fmt.Errorf("release ENQ failed: %w", err)
    }
    defer resp.Body.Close()
    io.ReadAll(resp.Body) // discard body

    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
        return fmt.Errorf("z/OSMF returned %d releasing ENQ for %s", resp.StatusCode, dsname)
    }
    return nil
}
```

- [ ] **Step 3: Verify `io` is imported in `internal/zosmf/datasets.go`**

The file already imports `"io"` (used in `ReadDataset`). Confirm it's present:

```bash
head -15 internal/zosmf/datasets.go
```

Expected: `"io"` in the import block. If missing, add it.

- [ ] **Step 4: Compile**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/zosmf/datasets.go
git commit -m "feat: add AcquireDatasetEnq and ReleaseDatasetEnq to zosmf Client"
```

---

## Task 2: Add MCP Tool Handlers for ENQ/DEQ

**Files:**
- Modify: `internal/tools/datasets.go`

- [ ] **Step 1: Add the `acquireDatasetEnqTool` struct and constructor at the end of `internal/tools/datasets.go`**

```go
// --- Acquire Dataset ENQ ---

type acquireDatasetEnqTool struct {
	clients *zosmf.ClientRegistry
}

type acquireDatasetEnqArgs struct {
	System  string `json:"system"`
	Dataset string `json:"dataset"`
	EnqType string `json:"enq_type"`
}

func NewAcquireDatasetEnqTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &acquireDatasetEnqTool{clients: clients}
}

func (t *acquireDatasetEnqTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_acquire_dataset_enq",
		Description: "Acquire an ENQ lock on a z/OS dataset or PDS member without reading its content. " +
			"Use enq_type 'exclusive' (SHRW) to prevent other readers and writers, or 'shared' (SHRE) to allow concurrent readers. " +
			"Returns a session_ref that must be passed to zosmf_release_dataset_enq when done, " +
			"and to zosmf_read_dataset / zosmf_write_dataset to associate operations with this ENQ session. " +
			"Returns an ENQ_NOT_AVAILABLE error if the dataset is already locked.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":  map[string]any{"type": "string", "description": "Name of the z/OS system"},
				"dataset": map[string]any{"type": "string", "description": "Dataset name, e.g. 'HLQ.PDS(MEMBER)' or 'HLQ.SEQ'"},
				"enq_type": map[string]any{
					"type":        "string",
					"description": "Lock type: 'exclusive' (SHRW — no other readers or writers) or 'shared' (SHRE — concurrent readers allowed)",
					"enum":        []string{"exclusive", "shared"},
				},
			},
			"required": []string{"system", "dataset", "enq_type"},
		}),
	}
}

func (t *acquireDatasetEnqTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args acquireDatasetEnqArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	sessionRef, err := client.AcquireDatasetEnq(args.Dataset, args.EnqType)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	data, _ := json.Marshal(map[string]string{"session_ref": sessionRef})
	return mcp.TextResult(string(data)), nil
}
```

- [ ] **Step 2: Add the `releaseDatasetEnqTool` struct directly after**

```go
// --- Release Dataset ENQ ---

type releaseDatasetEnqTool struct {
	clients *zosmf.ClientRegistry
}

type releaseDatasetEnqArgs struct {
	System     string `json:"system"`
	Dataset    string `json:"dataset"`
	SessionRef string `json:"session_ref"`
}

func NewReleaseDatasetEnqTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &releaseDatasetEnqTool{clients: clients}
}

func (t *releaseDatasetEnqTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_release_dataset_enq",
		Description: "Release a previously acquired ENQ lock on a z/OS dataset. Pass the session_ref returned by zosmf_acquire_dataset_enq.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":      map[string]any{"type": "string", "description": "Name of the z/OS system"},
				"dataset":     map[string]any{"type": "string", "description": "Same dataset name used when acquiring the ENQ"},
				"session_ref": map[string]any{"type": "string", "description": "Session reference returned by zosmf_acquire_dataset_enq"},
			},
			"required": []string{"system", "dataset", "session_ref"},
		}),
	}
}

func (t *releaseDatasetEnqTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args releaseDatasetEnqArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	if err := client.ReleaseDatasetEnq(args.Dataset, args.SessionRef); err != nil {
		// Non-fatal: log but don't error — ENQ will time out on z/OS
		return mcp.TextResult(fmt.Sprintf(`{"warning": "release ENQ failed (non-fatal): %s"}`, err.Error())), nil
	}

	return mcp.TextResult(`{}`), nil
}
```

- [ ] **Step 3: Compile**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/tools/datasets.go
git commit -m "feat: add zosmf_acquire_dataset_enq and zosmf_release_dataset_enq tool handlers"
```

---

## Task 3: Register the New Tools

**Files:**
- Modify: `internal/tools/registry.go`

- [ ] **Step 1: Add the two new tools to the `handlers` slice in `RegisterAll`**

Add them in the Dataset tools section, after `NewWriteDatasetTool(clients)`:

```go
// Dataset tools
NewListDatasetsTool(clients),
NewListMembersTool(clients),
NewReadDatasetTool(clients),
NewWriteDatasetTool(clients),
NewAcquireDatasetEnqTool(clients),
NewReleaseDatasetEnqTool(clients),
```

- [ ] **Step 2: Compile and verify tool count increases by 2**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Verify both tools appear in the tool list**

```bash
go run ./cmd/zosmf-mcp --version
```

Expected: prints version. Then run a quick check:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | go run ./cmd/zosmf-mcp --transport stdio --config /dev/null 2>/dev/null || true
```

Expected output contains `"zosmf_acquire_dataset_enq"` and `"zosmf_release_dataset_enq"` in the tools list. (The config error is acceptable — we only care the tool names appear.)

- [ ] **Step 4: Commit**

```bash
git add internal/tools/registry.go
git commit -m "feat: register zosmf_acquire_dataset_enq and zosmf_release_dataset_enq"
```

---

## Task 4: Update README with new tools

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add the two new tools to the Datasets section of the Tools Overview table**

Find the Datasets table in `README.md` (currently 6 tools) and add two rows:

```markdown
| `zosmf_acquire_dataset_enq` | Acquire an exclusive or shared ENQ lock on a dataset or member |
| `zosmf_release_dataset_enq` | Release a previously acquired ENQ lock |
```

Update the tool count in the section header from `6 tools` to `8 tools` and the total from `30 tools` to `32 tools`.

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add zosmf_acquire_dataset_enq and zosmf_release_dataset_enq to README"
```
