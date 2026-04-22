# ZOAU Integration Design

**Date:** 2026-04-22
**Status:** Approved

---

## Overview

Integrate 10 IBM Z Open Automation Utilities (ZOAU) CLI commands into the zosmf-mcp server as separate MCP tools. ZOAU commands run on z/OS USS via SSH — they are not available through the z/OSMF REST API. This requires a new SSH execution layer and SSH configuration per z/OS system.

---

## Requirements

- ZOAU >= 1.3.x must be installed on the target z/OS system. No automatic version check at server startup — this is a documented prerequisite.
- Only SSH key authentication is supported. Password-based SSH is not allowed.
- If no `ssh.key_file` is configured, SSH is invoked without `-i` (uses the system default `~/.ssh`).
- If SSH is not configured for a system, all ZOAU tools return a clear error message.
- All ZOAU tools are registered unconditionally — availability on a specific system is the operator's responsibility.

---

## Configuration Changes

New optional `ssh` block per system in `.zosmf-mcp-config.yaml`:

```yaml
systems:
  - name: "MAINT"
    host: "https://203.0.113.11:443"
    auth:
      type: "basic"
      username: "IBMUSER"
      password: "${ZOSMF_MAINT_PASSWORD}"
    ssh:
      host: "203.0.113.11"       # SSH hostname (may differ from z/OSMF host)
      port: 22                    # optional, default: 22
      username: "ibmuser"         # USS username (may differ from TSO username)
      key_file: "~/.ssh/id_rsa"  # optional — omit to use SSH default key
```

### New Config Fields (`SystemConfig`)

| Field          | Type   | Required | Default | Description                              |
|----------------|--------|----------|---------|------------------------------------------|
| `ssh.host`     | string | Yes*     | —       | SSH hostname (*required if ssh block present) |
| `ssh.port`     | int    | No       | 22      | SSH port                                 |
| `ssh.username` | string | Yes*     | —       | USS username for SSH login               |
| `ssh.key_file` | string | No       | —       | Path to SSH private key file             |

---

## Architecture

### New Files

- `internal/zosmf/ssh.go` — SSH execution layer
- `internal/tools/zoau.go` — 10 ZOAU MCP tool handlers

### Modified Files

- `internal/config/config.go` — add `SSHConfig` struct and `SSH` field to `SystemConfig`
- `internal/tools/registry.go` — register ZOAU tools

---

## SSH Execution Layer (`internal/zosmf/ssh.go`)

```go
type SSHConfig struct {
    Host     string
    Port     int
    Username string
    KeyFile  string
}

func RunSSH(cfg SSHConfig, command string) (string, error)
```

- Builds the SSH command:
  - With key file: `ssh -i <key_file> -p <port> -o BatchMode=yes -o StrictHostKeyChecking=accept-new <user>@<host> <command>`
  - Without key file: `ssh -p <port> -o BatchMode=yes -o StrictHostKeyChecking=accept-new <user>@<host> <command>`
- `BatchMode=yes` prevents password prompts — enforces key-only auth
- Timeout: 30 seconds via `context.WithTimeout`
- Returns stdout as string; returns error if exit code != 0 (includes stderr in error message)

---

## ZOAU MCP Tools (`internal/tools/zoau.go`)

All tools share the same pattern:
1. Resolve the system's SSH config — return error if not configured
2. Build the ZOAU command string
3. Call `RunSSH`
4. Attempt JSON parse if the command supports `-j`; otherwise return plain text

### Tool Reference

| MCP Tool | ZOAU Command | Client Parameters | `-j` support |
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

**Note:** `-j` support per command will be verified on the target system after SSH is implemented. Commands confirmed to support `-j` will use `command -j` and return parsed JSON; others return plain text.

### Parsing Strategy

- If the command supports `-j`: invoke with `-j`, parse JSON output, return as structured text
- Otherwise: return raw stdout as plain text
- In both cases: if the command fails (non-zero exit), return the stderr as an MCP error result

### All Tools — Common Arguments

| Parameter | Type   | Required | Description                    |
|-----------|--------|----------|--------------------------------|
| `system`  | string | Yes      | z/OS system name (as configured) |

### Tools with Additional Arguments

| Parameter | Type   | Required for tools           | Description           |
|-----------|--------|------------------------------|-----------------------|
| `member`  | string | `llwhence`, `parmwhence`, `procwhence` | Member name to locate |
| `pattern` | string | `parmgrep`, `procgrep`       | Search pattern        |

---

## Error Handling

| Condition | Behavior |
|-----------|----------|
| SSH not configured for system | MCP error: `"SSH not configured for system <name>"` |
| SSH connection fails | MCP error with SSH error details |
| ZOAU command not found (exit 127) | MCP error: `"ZOAU command not found — is ZOAU >= 1.3.x installed?"` |
| Command exits with non-zero | MCP error with stderr output |
| JSON parse fails (for -j commands) | Fall back to plain text output |

---

## Out of Scope

- Automatic ZOAU version check at server startup
- Password-based SSH
- SSH agent forwarding
- Known-hosts strict validation (uses `StrictHostKeyChecking=accept-new`)
