# MCP Resources and resource_link — Design

**Date:** 2026-08-17
**Status:** approved
**Scope:** Resources and `resource_link` only. Prompts and completions get their own cycles later.

## Problem

`zosmf-mcp` exposes tools and nothing else. Two consequences:

1. `zosmf_get_all_spools` (`internal/tools/jobs.go`) concatenates every spool file of a job with no size limit and returns it as one text result. A compile job with a large SYSPRINT pushes megabytes into the model's context window. `zosmf_read_dataset`, `zosmf_read_uss_file` and `zosmf_get_spool_content` have the same unbounded behaviour on a smaller scale.
2. There is no way for a user to attach a z/OS object to a conversation as addressable context. Every read goes through a model-initiated tool call.

Resources solve both: `resource_link` lets a tool return a pointer instead of a payload, and resource URIs give stable, user-attachable identity to datasets, USS files and spool output.

## Decisions

| Question | Decision |
| --- | --- |
| Protocol version | Raise to `2025-06-18` — `resource_link` requires it |
| Scope of this cycle | Resources and `resource_link`; prompts and completions later |
| Addressable objects | Dataset/PDS member, USS file, job spool file, system inventory |
| Relationship to read tools | Size threshold: small inline, large as preview + link |
| Clients without resource support | Always preview + link — one uniform behaviour |
| Access control | None beyond SAF/RACF, consistent with the existing tools |

## Architecture

New package `internal/resources/`, mirroring the structure of `internal/tools/`:

```
registry.go   ResourceRegistry, ResourceHandler, RegisterAll
uri.go        parse/build — also imported by internal/tools/
datasets.go   zosmf://{system}/dataset/{dsn}
ussfiles.go   zosmf://{system}/uss/{path}
jobspool.go   zosmf://{system}/job/{jobname}/{jobid}/spool/{id}
systems.go    zosmf:///systems
```

```go
type ResourceHandler interface {
    Definitions() []Resource
    Templates() []ResourceTemplate
    Matches(uri string) bool
    Read(ctx context.Context, uri string) (ResourceContents, error)
}
```

The registry routes by `Matches` and holds handlers behind an `RWMutex`, exactly as `ToolRegistry` does. `internal/tools/` imports `internal/resources/` for the URI builders; the dependency does not run the other way, so there is no cycle.

`internal/mcp/` gains the protocol side:

- `ProtocolVersion` → `2025-06-18`
- `ResourcesCapability` in `ServerCapabilities`
- types `Resource`, `ResourceTemplate`, `ResourceContents`
- `Content` extended with `uri`, `name`, `description`, `size` for `resource_link`
- dispatch for `resources/list`, `resources/templates/list`, `resources/read`

### URI scheme

System-scoped resources use the system name as authority, matching the `system` argument of the tools. Server-scoped resources use an empty authority, which avoids the collision `zosmf://systems` would have with a system actually named `SYSTEMS`.

```
zosmf://S0W1/dataset/SYS1.PARMLIB(IEASYS00)
zosmf://S0W1/uss/etc/profile
zosmf://S0W1/job/MYJOB/JOB12345/spool/2
zosmf:///systems
```

Parentheses are legal in URI paths (RFC 3986 sub-delims), so dataset names keep their native spelling. Everything after `/uss` is the absolute USS path.

Spool files are addressed by the numeric id z/OSMF assigns, not by DD name: several steps can share a DD name, so `SYSPRINT` alone is not unique. DD name and step name travel in the `name` and `description` of the `resource_link`, so the model still sees `JESYSMSG (step: GO)` when choosing.

`resources/list` returns exactly one entry, `zosmf:///systems`. Datasets, USS files and spool files cannot be enumerated — there is no meaningful "all datasets on this system" — so they appear only in `resources/templates/list`, as the three templates above.

## Data flow

One configuration value, `resources.inline_max_bytes`, default 32768. It caps how much content a tool result may carry as inline text.

Single-file tools — `zosmf_read_dataset`, `zosmf_read_uss_file`, `zosmf_get_spool_content`:

```
size ≤ threshold  →  text, as today
size >  threshold  →  preview (first 2 KB, cut at a line boundary)
                      + resource_link with uri, name, mimeType, size
```

The 2 KB preview is a constant, not a configuration value.

`zosmf_get_all_spools` changes behaviour:

1. fetch the spool directory — DD name, step name, size per file
2. inline files in the order z/OSMF returns them while the total stays within `inline_max_bytes`
3. once the budget is exhausted, emit `resource_link` only for the remaining files

A file that would not fit in the remaining budget is not partially inlined; it becomes a link. The one exception is the first file: if it alone exceeds the budget, it gets the same preview-plus-link treatment as the single-file tools, so a result is never link-only with no readable text.

A job with a small JESYSMSG, a small JESJCL and a large SYSPRINT therefore yields the two small files in full and the large one as a link — the shape wanted for failure analysis, since JES files come first in z/OSMF order.

Tools and `resources/read` call the same `internal/zosmf/` functions. Only the packaging differs: `ResourceContents` versus `[]Content`. No second implementation of the fetch path.

No caching. A dataset can change between reads and z/OSMF supplies no reliable validators, so every `resources/read` goes through to the system.

## Error handling

| Case | Behaviour |
| --- | --- |
| Unknown URI | JSON-RPC `-32002`, resource not found |
| Malformed URI | JSON-RPC `-32602` |
| z/OSMF 404 | "dataset not found" / "path not found" |
| z/OSMF 403 | "access denied by SAF/RACF" |
| Content not valid UTF-8 | `blob` with `mimeType: application/octet-stream` |
| Content over 10 MB | error naming the actual size |

z/OSMF errors are reduced to their statement. Response headers and request URLs are never passed through — they can carry credentials or internal hostnames.

The 10 MB ceiling on `resources/read` is a constant. `resources/read` has no threshold, because the client asked for the object deliberately, but an accidental read of a 400 MB dataset must not take the process down.

Resources carry no arguments, so the z/OSMF default conversion applies (text, IBM-1047). A specific code page is what `zosmf_read_dataset` with its `encoding` argument is for.

Dataset names are upper-cased when parsed, as the tools already do with system names. USS paths are left untouched, where case is significant.

`listChanged` stays off. The resource inventory is static — one resource, three templates — so there are no notifications and no subscriptions, and the server stays stateless with respect to resources.

## Testing

All handler tests use an `httptest` server as the z/OSMF end, following `internal/zosmf/client_auth_test.go` and `client_proxy_test.go`.

- **`uri_test.go`** — table-driven parse/build round-trips: DSN with and without member, USS paths with special characters, `zosmf:///systems`, missing segments, unknown scheme, case normalisation
- **`registry_test.go`** — registration, duplicate rejection, routing via `Matches`, unknown URI yields `-32002`
- **per handler** — success, 404, 403, binary content, the 10 MB ceiling
- **threshold logic** — below threshold plain text; above threshold preview plus link; preview cut at a line boundary; budget exhaustion in `get_all_spools` with mixed file sizes
- **protocol** — `initialize` advertises the resources capability; the three new methods dispatch correctly

The existing test for `zosmf_get_all_spools` in `internal/tools/` expects full concatenation and must be updated to the new behaviour.

## Compatibility

The server will advertise `2025-06-18`. A client requesting `2025-03-26` receives the version the server will speak; since the changes are additive, such clients work in practice. This is not something to assume — verify against n8n and at least one other older client after implementation, as an explicit step in the plan.

## Success criterion

A job whose SYSPRINT exceeds the threshold returns, via `zosmf_get_all_spools`, the small JES files in full and SYSPRINT as a `resource_link`; a subsequent `resources/read` on that link returns the complete content.
