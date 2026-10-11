# Contributing

Thanks for looking at this. The project is small and has a narrow scope: an
MCP server over the z/OSMF REST API, plus ZOAU over SSH.

## Before you start

- **No dependencies without a reason.** The module has two: `google/uuid` and
  `gopkg.in/yaml.v3`. A pull request that adds a third needs to say why the
  standard library is not enough.
- **Open an issue first for anything larger than a fix.** A new tool touches
  the schema every client sees, so it is worth agreeing on the shape before
  the code exists.
- **Nothing installation-specific in the repository.** No private IP
  addresses, internal host names, real user IDs or system names. Examples use
  the reserved ranges: `example.com` (RFC 2606) and `192.0.2.0/24`,
  `198.51.100.0/24`, `203.0.113.0/24` (RFC 5737). `make check-identifiers`
  enforces this and runs in CI.

## Building and testing

```bash
make build            # bin/zosmf-mcp with version and commit baked in
make test             # go test -race -count=1 ./...
make vet
make fmt              # gofmt -s
make lint             # golangci-lint, same version as CI
make check-identifiers
make build-all        # darwin/linux/windows, amd64/arm64/s390x
```

Always build through `make` — the Makefile sets the `-ldflags` that inject
the version and commit hash.

A single package: `go test -race -count=1 ./internal/mcp/`.

Tests read the real environment in a few places. If `MCP_API_KEY`,
`ZOSMF_PASSWORD` or `ZOSMF_PROXY` is exported in your shell, the affected
tests clear it themselves — but if you add a test that goes through
`config.Load`, clear what it depends on.

## Working without a z/OS system

Most changes can be verified without one:

- The z/OSMF client is exercised against `httptest` — see
  `internal/zosmf/client_http_test.go`.
- SSH and the ZOAU tools run against a stub `ssh` placed first on `PATH` —
  see `internal/zosmf/ssh_exec_test.go`.
- The HTTP transport's session handling is driven through
  `httptest.NewRecorder` — see `internal/mcp/transport_http_session_test.go`.

If a change can only be verified against a live system, say so in the pull
request and describe what you ran.

## Adding a tool

1. Implement `mcp.ToolHandler` (`Definition()` and `Execute()`) in the file
   for that domain under `internal/tools/`.
2. Register it in `RegisterAll` in `internal/tools/registry.go`.
3. Add the z/OSMF call to `internal/zosmf/` if one is missing.
4. Annotate it. `readOnlyHint` and `destructiveHint` decide whether a client
   asks the user before running it — a wrong annotation is a safety problem.
5. Document it in `docs/tools.md`, and list it in the README's warning
   section if it can destroy data.

## Error messages are an interface

Tool error strings are consumed by clients and automations. Markers like
`AUTH_FAILED(<system>)` and `NO_CREDENTIALS(<system>)` and the wording around
them are part of the contract. Do not reformat them without a reason, and
mention it in the changelog when you do.

## Pull requests

- One topic per pull request.
- `make fmt vet lint test` passes locally.
- Commits follow [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat:`, `fix:`, `docs:`, `test:`, `chore:`). A breaking change gets a `!`
  and a note in `CHANGELOG.md` under Unreleased.
- Write the commit body for someone reading it in a year: what broke, and
  why this fixes it.

## Security

Do not open a public issue for a vulnerability. See [SECURITY.md](SECURITY.md).
