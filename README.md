# zosmf-mcp

A [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server that lets Claude and other AI assistants work with z/OS — jobs, datasets, USS files, SMP/E, the operator console, TSO and parmlib through the z/OSMF REST API, plus ZOAU over SSH. You ask in plain language:

```
Show me all jobs in OUTPUT status on SYSA as a table
Is SYSMOD UA12345 installed in target zone TGTZONE on SYSA?
Which address spaces are active on SYSA right now?
```

> **⚠️ This server can change and destroy data on a live z/OS system,** with the full authority of the z/OS user you configure. Start with a test system, and read [Before You Start](#️-before-you-start) before you connect it to anything you care about.

## Quick start

You need z/OSMF on the target system (z/OS V2.3 or later), network access to its port, and an MCP client such as Claude Code or Claude Desktop.

### 1. Install

**macOS and Linux** — the example fetches `darwin-arm64` (Apple Silicon). For another platform, end the URL with `darwin-amd64` (Intel Mac), `linux-amd64` or `linux-s390x` instead.

```bash
curl -fsSLo zosmf-mcp https://github.com/cybersorcerer/zosmf-mcp/releases/latest/download/zosmf-mcp-darwin-arm64
install -d ~/.local/bin && install -m 755 zosmf-mcp ~/.local/bin/
export PATH="$HOME/.local/bin:$PATH"     # add this line to your shell profile to keep it
zosmf-mcp --version
```

Downloaded it with a browser instead? Then macOS refuses to run it; `xattr -d com.apple.quarantine zosmf-mcp` lifts that.

**Windows** — in PowerShell:

```powershell
Invoke-WebRequest https://github.com/cybersorcerer/zosmf-mcp/releases/latest/download/zosmf-mcp-windows.exe -OutFile zosmf-mcp.exe
```

Move `zosmf-mcp.exe` to a directory on your `PATH`.

Or [build it from source](#build-from-source).

### 2. Configure

```bash
zosmf-mcp init config   # the wizard: systems, CSIs, zones — it backs up the file first
zosmf-mcp init env      # the passwords — into a .env next to the configuration, mode 600
```

The wizard writes `~/.config/smpe_ls/.smpe-zosmf.yaml`, unless you already have a configuration elsewhere. The server reads the passwords from the `.env` itself; no client ever needs them. Every command and every key: [Configuration](docs/configuration.md).

### 3. Connect a client

For Claude Code — use the path `init config` printed if it differs:

```bash
claude mcp add --scope user zosmf-mcp -- zosmf-mcp --config ~/.config/smpe_ls/.smpe-zosmf.yaml
claude mcp list         # zosmf-mcp: ✔ Connected
```

Claude Desktop, VS Code, Cursor, Gemini CLI, JetBrains, Continue and n8n: [Connecting an MCP client](docs/clients.md).

Prefer an AI agent to walk you through this? `zosmf-mcp init skill --agent claude|opencode|pi` installs the same procedure as a skill.

## ⚠️ Before You Start

**This server lets an AI assistant change and destroy state on a live z/OS system.** It is not a read-only reporting tool. Point it at a sandbox or test LPAR first.

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

- **Never put passwords in the configuration file.** `zosmf-mcp init env` keeps them in a `.env` next to it that only you can read, and the server reads that file itself — see [Passwords](docs/configuration.md#passwords).
- **`rejectUnauthorized: false` (or `skip_tls_verify: true`) disables certificate validation.** It exists for self-signed test systems. Against a production system it exposes credentials to interception.
- **The HTTP transports listen on all interfaces by default and carry no TLS.** They refuse to start without an API key, but anyone who has the key, or can read the unencrypted traffic, drives the server with your z/OS credentials. Bind them to `127.0.0.1` or put a TLS reverse proxy in front — see [Transports and API key](docs/configuration.md#transports-and-api-key).
- **ZOAU tools authenticate by SSH key only.** The key grants shell access to the target system; protect it accordingly.

## Documentation

| Topic | Where |
| --- | --- |
| Setup commands, the configuration file, passwords, ZOAU, transports, forwarding | [docs/configuration.md](docs/configuration.md) |
| Connecting Claude Code, Claude Desktop, VS Code, Cursor, Gemini CLI, JetBrains, Continue, n8n — or your own client | [docs/clients.md](docs/clients.md) |
| Every tool with example prompts, and the MCP resources | [docs/tools.md](docs/tools.md) |
| Running the server in a container, with or without a jump host | [docs/container-setup.md](docs/container-setup.md) |
| Automation recipes | [docs/automations.md](docs/automations.md) |

## Build from source

You need Go 1.26.1 or later.

```bash
git clone https://github.com/cybersorcerer/zosmf-mcp.git
cd zosmf-mcp
make install            # builds and installs to ~/.local/bin/zosmf-mcp
```

`make help` lists every target; [CONTRIBUTING.md](CONTRIBUTING.md) covers building and testing in detail.

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
