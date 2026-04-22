# ZOAU Integration Design

**Date:** 2026-04-22
**Status:** Approved

---

## Overview

Integrate 10 IBM Z Open Automation Utilities (ZOAU) CLI commands into the zosmf-mcp server as separate MCP tools. ZOAU commands run on z/OS USS via SSH — they are not available through the z/OSMF REST API.

SSH connectivity and z/OSMF connectivity are orthogonal: a single z/OSMF endpoint can see multiple LPARs in a sysplex, but SSH requires a separate entry per LPAR. Therefore SSH systems are configured in a dedicated top-level `ssh_systems` section, independent of `systems`.

ZOAU tools are only registered if at least one `ssh_systems` entry is present in the configuration.

---

## Requirements

- ZOAU >= 1.3.x must be installed on the target z/OS system. No automatic version check at server startup — this is a documented prerequisite.
- Only SSH key authentication is supported. Password-based SSH is not allowed.
- If no `key_file` is configured for an SSH system, SSH is invoked without `-i` (uses the system default `~/.ssh`).
- If `ssh_systems` is empty or absent, ZOAU tools are not registered.
- If `system` in a tool call does not match any `ssh_systems` entry, the tool returns a clear error.

---

## Configuration

New top-level `ssh_systems` section in `.zosmf-mcp-config.yaml`, independent of `systems`:

```yaml
systems:
  - name: "MAINT"
    host: "https://203.0.113.11:443"
    auth:
      type: "basic"
      username: "IBMUSER"
      password: "${ZOSMF_MAINT_PASSWORD}"

ssh_systems:
  - name: "MVSA"
    host: "mvsa.example.com"
    port: 22                    # optional, default: 22
    username: "ibmuser"
    key_file: "~/.ssh/id_rsa"  # optional — omit to use SSH default key
  - name: "MVSB"
    host: "mvsb.example.com"
    username: "ibmuser"
```

### SSH System Fields

| Field      | Type   | Required | Default | Description                                   |
|------------|--------|----------|---------|-----------------------------------------------|
| `name`     | string | Yes      | —       | LPAR name — used as `system` in tool calls    |
| `host`     | string | Yes      | —       | SSH hostname or IP                            |
| `port`     | int    | No       | 22      | SSH port                                      |
| `username` | string | Yes      | —       | USS username for SSH login                    |
| `key_file` | string | No       | —       | Path to SSH private key; omit for SSH default |

---

## Architecture

### New Files

- `internal/zosmf/ssh.go` — SSH execution layer and `SSHClientRegistry`
- `internal/tools/zoau.go` — 10 ZOAU MCP tool handlers

### Modified Files

- `internal/config/config.go` — add `SSHSystemConfig` struct and `SSHSystems []SSHSystemConfig` to `Config`
- `internal/tools/registry.go` — conditionally register ZOAU tools if `ssh_systems` is non-empty
- `cmd/zosmf-mcp/main.go` — pass SSH registry to tool registration

---

## SSH Execution Layer (`internal/zosmf/ssh.go`)

```go
type SSHSystemConfig struct {
    Name     string
    Host     string
    Port     int
    Username string
    KeyFile  string
}

type SSHClientRegistry struct {
    systems map[string]SSHSystemConfig
}

func NewSSHClientRegistry(systems []SSHSystemConfig) *SSHClientRegistry
func (r *SSHClientRegistry) Get(name string) (SSHSystemConfig, error)
func RunSSH(cfg SSHSystemConfig, command string) (string, error)
```

`RunSSH` builds the SSH command:

- With key file: `ssh -i <key_file> -p <port> -o BatchMode=yes -o StrictHostKeyChecking=accept-new <user>@<host> <command>`
- Without key file: `ssh -p <port> -o BatchMode=yes -o StrictHostKeyChecking=accept-new <user>@<host> <command>`

`BatchMode=yes` prevents password prompts — enforces key-only auth.
Timeout: 30 seconds via `context.WithTimeout`.
Returns stdout as string; returns error if exit code != 0 (stderr included in error message).

---

## ZOAU MCP Tools (`internal/tools/zoau.go`)

All tools share the same pattern:
1. Look up `system` in the `SSHClientRegistry` — return error if not found
2. Build the ZOAU command string
3. Call `RunSSH`
4. If the command supports `-j`: invoke with `-j` and return parsed JSON as text; otherwise return plain text

### Tool Reference

| MCP Tool | ZOAU Command | Extra Parameters | `-j` support |
|---|---|---|---|
| `zosmf_zoau_pll` | `pll -v` | — | TBD |
| `zosmf_zoau_zinfo` | `zinfo -a` | — | TBD |
| `zosmf_zoau_llwhence` | `llwhence -v <member>` | `member` (string) | TBD |
| `zosmf_zoau_parmgrep` | `parmgrep -n -v <pattern>` | `pattern` (string) | TBD |
| `zosmf_zoau_parmwhence` | `parmwhence -v <member>` | `member` (string) | TBD |
| `zosmf_zoau_pproc` | `pproc -v` | — | TBD |
| `zosmf_zoau_procgrep` | `procgrep -n -v <pattern>` | `pattern` (string) | TBD |
| `zosmf_zoau_procwhence` | `procwhence -v <member>` | `member` (string) | TBD |
| `zosmf_zoau_version` | `zoaversion -v` | — | TBD |

**Note:** `-j` support per command will be verified on the target system after SSH is implemented. Commands confirmed to support `-j` will be invoked with `-j` and return parsed JSON; others return plain text.

### Common Tool Parameter

| Parameter | Type   | Required | Description                              |
|-----------|--------|----------|------------------------------------------|
| `system`  | string | Yes      | LPAR name as defined in `ssh_systems`    |

### Additional Parameters

| Parameter | Type   | Required for                           | Description           |
|-----------|--------|----------------------------------------|-----------------------|
| `member`  | string | `llwhence`, `parmwhence`, `procwhence` | Member name to locate |
| `pattern` | string | `parmgrep`, `procgrep`                 | Search pattern        |

---

## Tool Registration

In `internal/tools/registry.go`:

```go
if len(cfg.SSHSystems) > 0 {
    sshRegistry := zosmf.NewSSHClientRegistry(cfg.SSHSystems)
    // register all 10 ZOAU tools with sshRegistry
}
```

---

## Error Handling

| Condition | Behavior |
|-----------|----------|
| `ssh_systems` absent or empty | ZOAU tools not registered |
| `system` not found in registry | MCP error: `"SSH system not found: <name>"` |
| SSH connection fails | MCP error with SSH error details |
| ZOAU command not found (exit 127) | MCP error: `"ZOAU command not found — is ZOAU >= 1.3.x installed and in PATH?"` |
| Command exits non-zero | MCP error with stderr output |
| JSON parse fails (for -j commands) | Fall back to plain text output |

---

## Out of Scope

- Automatic ZOAU version check at server startup
- Password-based SSH
- SSH agent forwarding
- Known-hosts strict validation (uses `StrictHostKeyChecking=accept-new`)
