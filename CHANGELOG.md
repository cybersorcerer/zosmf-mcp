# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Entries for releases up to 0.5.2 were reconstructed from the commit history.

## [Unreleased]

### Added

- **MCP resources.** Datasets, USS files, job spool files and the configured
  system inventory are exposed as resources with `zosmf://` URIs, alongside
  the existing tools. Protocol version raised to `2025-06-18`.
- **Output budgeting.** A tool result that would exceed the configured
  `resources.inline_max_bytes` returns a preview plus a `resource_link`
  instead of the full content; clients fetch the rest through
  `resources/read`. Oversized spool files are linked without being fetched.
- **Bearer token authentication** for both HTTP transports. An HTTP transport
  refuses to start without an `api_key` unless `allow_insecure: true` is set.
- **Request forwarding.** `proxy` and `ssh_proxy_jump` settings route z/OSMF
  REST calls through a SOCKS/HTTP proxy and ZOAU SSH through a jump host,
  with a per-system override.
- **Container setup.** Dockerfile, Compose file and `make container-*`
  targets for Apple container, documented in `docs/container-setup.md`.
- **Tool annotations** (`readOnlyHint`, `destructiveHint`, …) on every tool,
  plus a system enum in the input schemas.
- **`zosmf_set_password`** so a system configured without a password can be
  supplied one at runtime.
- **Request logging** with duration and result size; rejected requests on the
  HTTP transports are logged with the reason.
- Server-side dataset search through `?search=` in `zosmf_read_dataset`.
- Config file discovery: `.smpe-zosmf.yaml` in the working directory, then
  `~/.config/smpe_ls/`, with the legacy name as a fallback.
- `SECURITY.md`, `CONTRIBUTING.md` and a lint job in CI.

### Changed

- **Breaking:** ZOAU tools moved to the `zoau_` namespace and take an
  `ssh_system` parameter instead of `system`.
- A 401 from z/OSMF is marked `AUTH_FAILED(<system>)`, a missing credential
  `NO_CREDENTIALS(<system>)`, so a client can tell them apart.
- Every response body read is bounded at 10 MB.

### Fixed

- The ProxyJump hop inherits neither `-i` nor `BatchMode` from `ssh -J`, so
  ZOAU tools failed with "Too many authentication failures" on a host without
  a default identity — a container. The jump hop now runs through an explicit
  `ProxyCommand` carrying both.
- `ZOSMF_PROXY` alone enables the proxy; no `proxy:` block is required.
- `resources/read` maps a missing dataset to JSON-RPC `-32002` instead of a
  generic internal error.
- USS and dataset paths are percent-encoded in resource URIs.
- Unresolved `${VAR}` references in the configuration abort startup and name
  the variable, instead of reaching z/OSMF as literal text.

## [0.5.2] — 2026-05-01

### Added

- `volser` and `start` for dataset listing, `release_enq` for writes, and USS
  file tag support.

## [0.5.1] — 2026-04-27

### Added

- Streamable HTTP transport for the n8n MCP Client node.

### Fixed

- SYSLOG query behavior aligned with `zcli`.

## [0.5.0] — 2026-04-23

### Added

- ZOAU tools over SSH, registered only when `ssh_systems` is configured:
  `pll`, `zinfo`, `llwhence`, `parmgrep`, `parmwhence`, `pproc`, `procgrep`,
  `procwhence`, `vtocls` and arbitrary command execution.
- `environment` map per SSH system for the ZOAU path setup.
- `zosmf_get_all_spools`.

### Fixed

- HTTP transport keeps session state per connection.
- `zones` accepted as an alias for `target_zones`, for `smpe_ls` config
  compatibility.
- SSH commands are shell-quoted.

## [0.4.0] — 2026-04-20

### Added

- Dataset ENQ handling: `zosmf_acquire_dataset_enq` and
  `zosmf_release_dataset_enq`; the acquire call returns the content, so no
  separate read is needed.
- `encoding` parameter for reading and writing datasets.
- Additional query parameters for job, dataset and SMP/E queries.

### Fixed

- `X-IBM-Attrs` header instead of a query parameter to request dataset and
  member attributes.
- Multi-zone CSI queries in a single API call.
- Struct fields aligned with the z/OSMF response, which returns all dataset
  attributes as strings.

## [0.3.0] — 2026-04-09

### Added

- Parmlib service.

## [0.2.0] — 2026-04-05

### Added

- First tagged release: MCP server over stdio and HTTP with tools for jobs,
  datasets, filesystems, USS files, console commands, TSO and SMP/E.
