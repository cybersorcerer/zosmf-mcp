# zosmf-mcp — User Guide

A [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server for z/OS that gives Claude and other AI assistants direct access to z/OS systems via the z/OSMF REST API.


## ⚠️ Before You Start

**This server lets an AI assistant change and destroy state on a live z/OS system.** It is not a read-only reporting tool. Point it at a sandbox or test LPAR first, and read this section before connecting it to anything you care about.

### Tools that can destroy data or interrupt running work

These are annotated with `destructiveHint: true`, which well-behaved MCP clients surface as a confirmation prompt — but **the annotation is advisory. Nothing in the protocol forces a client to ask.**

| Tool | What it can do |
| --- | --- |
| `zosmf_delete_dataset` | Deletes a dataset or PDS member |
| `zosmf_write_dataset` | Overwrites dataset content in place |
| `zosmf_delete_uss_file` | Deletes a USS file or directory |
| `zosmf_write_uss_file` | Overwrites USS file content in place |
| `zosmf_delete_zfs` | Deletes a zFS aggregate |
| `zosmf_unmount_filesystem` | Unmounts a filesystem out from under running work |
| `zosmf_cancel_job` | Cancels a running job |
| `zosmf_purge_job` | Purges a job and its output |

### Tools with unbounded reach

Three tools pass an arbitrary command through to the system. Their effect is limited only by the permissions of the configured user — not by anything this server enforces:

| Tool | Passes through to |
| --- | --- |
| `zosmf_issue_command` | MVS operator console |
| `zosmf_tso_command` | TSO |
| `zoau_exec` | USS shell, via SSH |

An operator command can take down a subsystem. A TSO or USS command can do anything the userid is allowed to do. Treat these as equivalent to handing out a console or shell session.

### The permissions you grant are the real boundary

This server enforces no authorization of its own. Every limit comes from SAF/RACF on the configured userid. An AI assistant asked to "clean up old datasets" will do exactly as much damage as that userid permits.

- Give the userid the **minimum** SAF/RACF permissions the tools you actually use require.
- Do not reuse a systems-programmer or otherwise privileged userid.
- Prefer a userid whose access is scoped to a dedicated HLQ.

### Network and credential exposure

- **Never hardcode passwords.** Use `${ENV_VAR}` substitution or the `ZOSMF_PASSWORD` environment variable.
- **`skip_tls_verify: true` disables certificate validation.** It exists for self-signed test systems. Setting it against a production system exposes credentials to interception.
- **The HTTP and Streamable HTTP transports listen on all interfaces by default.** Anyone who can reach the port can drive the server with your z/OSMF credentials. Bind to localhost, put it behind a reverse proxy, or enable API-key authentication — see [API-key authentication](docs/configuration.md#api-key-authentication).
- **ZOAU tools authenticate by SSH key only.** The key grants shell access to the target system; protect it accordingly.


## Prerequisites

- **Go 1.22+** — to build from source
- **z/OSMF** — enabled on the target system (z/OS V2.3+)
- **Network access** — to the z/OSMF port (default: 443)
- An **MCP-compatible AI client** — Claude Desktop, Claude Code CLI, VS Code, Cursor, etc.


## Getting started

Download the binary for your platform from the [latest release](https://github.com/cybersorcerer/zosmf-mcp/releases/latest) — `darwin-arm64`, `darwin-amd64`, `linux-amd64`, `linux-s390x` or `windows.exe` — or [build from source](#build-from-source):

```bash
curl -fsSLo zosmf-mcp https://github.com/cybersorcerer/zosmf-mcp/releases/latest/download/zosmf-mcp-darwin-arm64
install -d ~/.local/bin && install -m 755 zosmf-mcp ~/.local/bin/ && export PATH="$HOME/.local/bin:$PATH"
zosmf-mcp init config   # interactive: systems, CSIs, zones — backs up the file first
zosmf-mcp init env      # that system's password — writes .env next to the config, mode 600
```

Register with an MCP client, for example Claude Code — `claude mcp list` should then show `zosmf-mcp` as `✔ Connected`:

```bash
claude mcp add zosmf-mcp -- zosmf-mcp --config <path init config printed>
```

The server reads the `.env` next to its configuration at startup, so no client needs the password. `zosmf-mcp init config --check` shows where each system's password comes from.

All `init` commands and the wizard: [Setup Commands](docs/configuration.md#setup-commands). More systems, other clients, and containers: [docs/configuration.md](docs/configuration.md) and [docs/clients.md](docs/clients.md) — the wizard automates the same configuration file those sections describe editing by hand. Prefer an agent to run this for you? `zosmf-mcp init skill --agent claude|opencode|pi` installs the same procedure as a skill (`--scope project` for the current directory only).


## Installation

### Build from Source

```bash
git clone https://github.com/cybersorcerer/zosmf-mcp.git
cd zosmf-mcp
make install
```

The binary is installed to `~/.local/bin/zosmf-mcp`. Make sure this directory is on your `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

### Available Make Targets

| Target    | Description                              |
|-----------|------------------------------------------|
| `build`   | Build the binary                         |
| `install` | Install binary to `~/.local/bin`         |
| `test`    | Run tests with race detector             |
| `vet`     | Run `go vet`                             |
| `fmt`     | Format source code                       |
| `lint`    | Run `golangci-lint`                      |
| `clean`   | Remove binary and caches                 |
| `tidy`    | Run `go mod tidy`                        |


## Documentation

| Topic | Where |
| --- | --- |
| Configuration file, passwords, SSH and ZOAU, proxy, API key | [docs/configuration.md](docs/configuration.md) |
| Registering with Claude Desktop, Claude Code, VS Code, JetBrains, Gemini CLI, Continue, Cursor, n8n, custom clients | [docs/clients.md](docs/clients.md) |
| Every tool with example prompts, and the MCP resources | [docs/tools.md](docs/tools.md) |
| Running the server in a container | [docs/container-setup.md](docs/container-setup.md) |
| Automation recipes | [docs/automations.md](docs/automations.md) |
## Contributing and security

Pull requests are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) for how to
build, test and add a tool. Release history is in [CHANGELOG.md](CHANGELOG.md).

Report a vulnerability privately through GitHub's
[private vulnerability reporting](https://github.com/cybersorcerer/zosmf-mcp/security/advisories/new),
not as a public issue. [SECURITY.md](SECURITY.md) describes what counts as
one and which deployment risks are documented behavior.

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for the full text.

Copyright 2026 Ronny Funk (Cybersorcerer)
