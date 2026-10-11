# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Entries for releases up to 0.5.2 were reconstructed from the commit history.

## [Unreleased]

### Added

- **The server reads the `.env` beside its configuration** at startup, so
  no MCP client has to be given a password. The process environment still
  wins; a `.env` others can read is ignored with a warning.
- **`init config --show`** prints the systems already in the file as JSON,
  keyed by the flag names, without secrets.
- **`init config --check` names each system's password source**, so a
  setup can prove the password arrives without starting the server.

### Changed

- **`init config --print` shows only the system being written**, not the
  whole shared file. `init config` says whether it added or updated it.
- **`agents/setup.md` rewritten** so it works with small local models: the
  user runs the wizard and `init env` in their own terminal; the agent only
  checks the result with `--show` and `--check` and connects the client. It
  never types a setting and never sees a password.

### Fixed

- **`zosmf_tso_command` ignored `tso_account`, `tso_proc`, `tso_charset` and
  `tso_codepage`** — it required the first two and never sent any of them.
  With `tso_account` set, a command now runs in its own TSO address space with
  exactly these settings (defaults `IZUFPROC`, `697`, `1047`); without it,
  z/OSMF chooses the address space as before, and nothing is required. The
  errors `has no tso_account configured` and `has no tso_proc configured` are
  gone; a procedure, character set or code page without an account is now an
  error. **Placeholder values in a configuration now take effect — correct or
  remove them.**
- **A z/OSMF host without `https://`** was written and passed `--check`,
  then failed every call. `init config` now rejects it, `--check` fails on
  it, and the server warns at startup.
- **The wizard kept an existing host without `https://`** when Enter was
  pressed; it now asks for a valid one.
- **`init env` asked again for every password** already in `.env`; it now
  keeps them, and `--questions` lists only what is missing.

## [1.0.0] — 2026-10-01

### Added

- **`zosmf-mcp init config` and `zosmf-mcp init env`.** An interactive wizard
  — a menu over the file's systems, CSIs and zones entered one per line, a
  preview before each write — or flags for scripting, that write the shared configuration file and its
  `.env` secrets — kept next to the configuration file, not in the working
  directory: one system's connection details, its ZOAU/SSH settings as
  one all-or-nothing group, and the `MCP_API_KEY` or password variables the
  file ends up needing. `--questions` prints the field catalog as JSON for an
  agent to drive the wizard from; `init config` also has `--check` and
  `--print` to validate or preview a file without writing it, and `init env`
  has `--generate-api-key` to mint `MCP_API_KEY` instead of asking for it.
  Neither command hangs when there is no terminal and no flags — it names
  what's missing and exits. A file `init config` writes with no transport
  block at all gets `transport: stdio: true`, so it can start the server
  unmodified.
- **Backups and `zosmf-mcp init restore`.** Every write by `init config` or
  `init env` first copies the file into `.zosmf-mcp-backups/` next to it,
  keeping the newest 10. `init restore --list` and `--backup <timestamp>` put
  one back; the wizard offers the same as `[r]`.
- **Per-system password lookup by convention.** A password not given in the
  file is read from `ZOSMF_<SYSTEM>_PASSWORD` — derived from the system's
  `name`, nothing to add to the file for this one — before falling back to the
  existing global `ZOSMF_PASSWORD`. The startup log names which of the three
  sources each system used, never the value.
- **`agents/setup.md`**, a setup procedure meant to be followed identically by
  Claude Code, opencode, pi, or a human at a terminal. It is built into the
  binary: `zosmf-mcp init skill --agent claude|opencode|pi [--scope
  user|project]` installs it, and a matching wrapper, wherever that agent
  looks for skills — no repository needed.

## [0.6.0] — 2026-09-21

The ZOAU tool names changed in this release — see **Changed** below.

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
- **Containers without a jump host.** `make docker-up-direct` and
  `make container-up-direct` run the server alone on a host that reaches
  z/OSMF itself; the tunnel service moved behind a Compose profile.
- **Autostart on macOS.** `make launch-agent` installs a LaunchAgent that
  starts the container at login, since Apple container has no restart policy.
- `SECURITY.md`, `CONTRIBUTING.md`, `CHANGELOG.md`, issue and pull request
  templates, and a lint job in CI.
- Tests for the paths that previously needed a live z/OS: SSH execution
  against a stub `ssh`, the z/OSMF client against `httptest`, HTTP session
  handling, and the ZOAU tool seam.

### Changed

- **Breaking:** ZOAU tools moved to the `zoau_` namespace and take an
  `ssh_system` parameter instead of `system`.
- A 401 from z/OSMF is marked `AUTH_FAILED(<system>)`, a missing credential
  `NO_CREDENTIALS(<system>)`, so a client can tell them apart.
- Every response body read is bounded at 10 MB.
- `--version` now prints the layout the rest of the suite uses: name and
  version on one line, `Commit:` below it.

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
- `serverInfo.version` was a constant last updated at 0.3.0, so every release
  since announced 0.3.0 to its clients. It now carries the build version, and
  `--version` and `serverInfo` report the same string.
- A containerised build reported `docker` as its version; the image now gets
  the real one as a build argument.
- `make container-down` no longer reports an ignored error on a host that has
  no tunnel container.
- 21 discarded write and `Close` errors, two untagged switches and a nil
  context in a test, all found by the new lint job.

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
