# ENQ/DEQ Dataset Tools Design

**Goal:** Add two new MCP tools to `zosmf_mcp` that allow callers to acquire and release z/OS dataset ENQ locks, enabling safe concurrent dataset access from editor clients like `vscpdf`.

**Architecture:** Two new tools in `internal/tools/datasets.go` alongside the existing `zosmf_read_dataset` and `zosmf_write_dataset`. The tools map directly to z/OSMF REST API headers for ENQ control. No new files are needed.

**Tech Stack:** Go 1.22+, z/OSMF REST API (`/zosmf/restfiles/ds/`), existing `zosmf` client infrastructure.

---

## Background

The z/OSMF dataset REST API supports ENQ (enqueue) control via HTTP headers:

- `X-IBM-Obtain-ENQ: SHRW` — exclusive lock (one writer, no other readers or writers)
- `X-IBM-Obtain-ENQ: SHRE` — shared lock (multiple readers, no writers)
- `X-IBM-Session-Ref: <ref>` — carries the session reference returned when ENQ was acquired
- Release: `DELETE /zosmf/restfiles/ds/<dataset>` with `X-IBM-Session-Ref` header

The session reference (`X-IBM-Session-Ref`) is returned in the response header when ENQ is acquired and must be passed back on subsequent operations (read, write, release) to tie them to the same ENQ session.

---

## New Tools

### `zosmf_acquire_dataset_enq`

Acquires an ENQ lock on a dataset or PDS member without reading its content. The caller is responsible for reading content separately via `zosmf_read_dataset`.

**Input schema:**
```json
{
  "system":   "string  — name of the z/OS system",
  "dataset":  "string  — dataset name, e.g. 'HLQ.PDS(MEMBER)' or 'HLQ.SEQ'",
  "enq_type": "string  — 'exclusive' (SHRW) or 'shared' (SHRE)"
}
```

**Output:**
```json
{ "session_ref": "string" }
```

**HTTP mapping:**
- `GET /zosmf/restfiles/ds/<dataset>` with:
  - `X-IBM-Obtain-ENQ: SHRW` (exclusive) or `X-IBM-Obtain-ENQ: SHRE` (shared)
  - `X-IBM-Data-Type: text` (to satisfy z/OSMF — content is discarded)
- Response header `X-IBM-Session-Ref` is extracted and returned
- Response body is discarded (caller reads content separately)

**Error cases:**
- ENQ not available (another holder): return MCP error with message prefix `ENQ_NOT_AVAILABLE` so callers can detect and surface a meaningful message
- Invalid `enq_type` value: return MCP error `invalid enq_type: must be 'exclusive' or 'shared'`
- Dataset not found: propagate z/OSMF 404 as MCP error

---

### `zosmf_release_dataset_enq`

Releases a previously acquired ENQ lock.

**Input schema:**
```json
{
  "system":      "string  — name of the z/OS system",
  "dataset":     "string  — same dataset name used when acquiring",
  "session_ref": "string  — session reference returned by zosmf_acquire_dataset_enq"
}
```

**Output:**
```json
{}
```

**HTTP mapping:**
- `DELETE /zosmf/restfiles/ds/<dataset>` with:
  - `X-IBM-Session-Ref: <session_ref>`
- Accepts HTTP 200 and 204 as success
- Errors are logged but not fatal — a failed release should not block the caller (ENQ will eventually time out on z/OS)

---

## File Changes

| File | Action | What changes |
|------|--------|-------------|
| `internal/tools/datasets.go` | Modify | Add `acquireDatasetEnqTool` and `releaseDatasetEnqTool` structs + registration |
| `internal/tools/registry.go` | Modify | Register the two new tools in `RegisterAll` |
| `internal/zosmf/datasets.go` | Modify | Add `AcquireDatasetEnq(dataset, enqType)` and `ReleaseDatasetEnq(dataset, sessionRef)` methods to the zosmf client |

---

## Design Decisions

**Why acquire-only, not acquire+read?**
Separating ENQ acquisition from reading keeps tools single-responsibility and allows callers to choose when and how to read (e.g. with specific encoding options). It also matches how `vscpdf`'s `DatasetEditor` works today.

**Why discard the response body on acquire?**
z/OSMF requires a GET to acquire ENQ, but the caller reads content as a separate step. Discarding avoids double-buffering large datasets.

**Why prefix ENQ_NOT_AVAILABLE in the error message?**
`vscpdf`'s `DatasetEditor` detects this prefix to show a specific "locked by another user" banner rather than a generic error. This convention is already established in the existing codebase.

**Why is release non-fatal on error?**
ENQ releases are best-effort. If the extension crashes or the connection drops, z/OS will time out the ENQ automatically. Surfacing release errors to the user would be confusing and unhelpful.
