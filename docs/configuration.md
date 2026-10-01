# Configuration

Everything the server reads from its configuration file and the environment. `zosmf-mcp init config` and `zosmf-mcp init env` write the same file and variables this page describes.

[← README](../README.md)


Configuration is done via a YAML file. By default the server searches for configuration in the following order (first match wins):

1. Path specified via `--config` flag
2. `.smpe-zosmf.yaml` in the current working directory
3. `.smpe-zosmf.yaml` in `~/.config/smpe_ls/`
4. `.zosmf-mcp-config.yaml` in the current working directory **(deprecated)**

> **Note:** The `.zosmf-mcp-config.yaml` file is deprecated. For new installations, use `.smpe-zosmf.yaml`. This shared configuration file is used by zosmf-mcp, vscode-smpe, vscpdf, and nvpdf.nvim — all four tools ignore unknown fields in the config.

## Canonical Configuration Example

The following is the recommended shared configuration file `.smpe-zosmf.yaml`, which is read by zosmf-mcp, vscode-smpe, vscpdf, and nvpdf.nvim:

```yaml
# .smpe-zosmf.yaml — shared by zosmf-mcp, vscode-smpe, vscpdf and nvpdf.nvim
servers:
  - name: PROD
    host: https://zosmf.example.com
    port: 443
    user: USER1                  # z/OSMF userid (basic auth)
    rejectUnauthorized: true
    # auth:                      # alternative to user:, with env expansion
    #   username: USER1
    #   password: ${PROD_PASSWORD}
    # tso_account: ACCT1         # required for zosmf_tso_command
    # tso_proc: IKJACCNT
    # csi:                       # SMP/E (used by vscode-smpe and zosmf_smpe_query_csi)
    #   - SMPE.GLOBAL.CSI
    # zones: [GLOBAL, TGT1]
    # defaultZones: [GLOBAL]
    # home: /u/user1             # USS home (used by vscpdf/nvpdf)

# ssh_systems:                   # optional, for ZOAU tools via SSH
#   - name: PROD
#     host: prod.example.com
#     port: 22
#     username: user1
#     key_file: ~/.ssh/id_rsa

# defaultServer: PROD            # used by vscode-smpe; ignored by zosmf-mcp
```

## Compatibility Aliases

The following fields are accepted as aliases for compatibility with other z/OSMF tools:

| Alias                | Equivalent to                 |
|----------------------|-------------------------------|
| `servers`            | `systems`                     |
| `user`               | `auth.username`               |
| `rejectUnauthorized` | `skip_tls_verify` (inverted)  |

This means you can use the simpler `servers:` key with `user:` and `rejectUnauthorized:` fields (as shown in the canonical example above) instead of the older nested `systems` list with `auth` blocks.

## Configuration Reference

| Field             | Required | Description                                                       |
|-------------------|----------|-------------------------------------------------------------------|
| `name`            | Yes      | System name used in prompts (e.g. "PROD", "MAINT")               |
| `host`            | Yes      | z/OSMF base URL including port                                    |
| `port`            | No       | Port number — appended to `host` if not already present          |
| `auth.type`       | No       | `basic` or `certificate` — defaults to `basic` when a username is present; entries without any credentials start but reject tool calls |
| `auth.username`   | Yes*     | TSO user ID (*required for `basic`)                               |
| `auth.password`   | Yes*     | Password — reference via `${ENV_VAR}` to avoid hardcoding        |
| `auth.cert_file`  | Yes**    | PEM certificate file (**required for `certificate`)               |
| `auth.key_file`   | Yes**    | PEM private key file (**required for `certificate`)               |
| `ca_file`         | No       | PEM file with the CA certificate for self-signed z/OSMF certs    |
| `skip_tls_verify` | No       | Skip TLS validation — not recommended for production             |
| `csi`             | No       | Default CSI datasets for SMP/E queries (list of dataset names)   |
| `zones`           | No       | Default SMP/E zones (used when no zone is specified in tool call) |
| `defaultZones`    | No       | Default SMP/E zones used by vscode-smpe; ignored by zosmf-mcp     |
| `tso_account`     | No       | TSO account number (required for `zosmf_tso_command`)            |
| `tso_proc`        | No       | TSO logon procedure (required for `zosmf_tso_command`)           |
| `tso_charset`     | No       | TSO character set, default: `697`                                 |
| `tso_codepage`    | No       | TSO code page, default: `1047`                                    |
| `home`            | No       | USS home directory, used by vscpdf/nvpdf                          |

## Passing Passwords Securely

Never store passwords in plain text in the configuration file. Use environment variables instead:

```bash
# .env file (do NOT commit to git!)
export ZOSMF_SYSA_PASSWORD="my_password"

# Load before starting:
source .env
```

In the config:
```yaml
password: "${ZOSMF_SYSA_PASSWORD}"
```

A system's password is resolved in this order: an explicit `${ENV_VAR}` (or literal value) in `auth.password`; otherwise `ZOSMF_<SYSTEM>_PASSWORD`, derived from the system's `name` — nothing to add to the file for this one; otherwise the global `ZOSMF_PASSWORD` fallback. The startup log names which of the three each system used, never the value itself.

## SSH Systems (for ZOAU tools)

ZOAU tools require SSH access to individual LPARs. SSH systems are configured independently of z/OSMF systems — one z/OSMF endpoint can serve an entire sysplex, while SSH requires a separate entry per LPAR.

```yaml
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

ZOAU tools are only registered when at least one `ssh_systems` entry is present. Password-based SSH is not supported — only key authentication.

### SSH Systems Configuration Reference

| Field      | Required | Default | Description                                   |
|------------|----------|---------|-----------------------------------------------|
| `name`     | Yes      | —       | LPAR name — used as `system` in tool calls    |
| `host`     | Yes      | —       | SSH hostname or IP address                    |
| `port`     | No       | 22      | SSH port                                      |
| `username` | Yes      | —       | USS username for SSH login                    |
| `key_file` | No       | —       | Path to SSH private key; omit for SSH default |


## Request Forwarding, API-Key Authentication and Docker

By default zosmf-mcp needs direct network access to the z/OSMF REST API (and, for ZOAU
tools, direct SSH access to the target LPARs). This section is for the case where the
server itself runs somewhere without that access — for example in a container on a
homelab Docker host that can only reach the z/OS systems indirectly, through another
machine that does have a route to them.

### Topology

```text
MacBook (privat)          Homelab-Dockerhost           Maschine 2                z/OS
──────────────────        ────────────────────         ──────────────            ──────
Claude Code / Agent  ──▶  zosmf-mcp ──▶ tunnel   ──▶   sshd (Jump)   ──▶  z/OSMF REST
                 (MCP über HTTP)   (SOCKS5)      (SSH)              (HTTPS)
```

A tunnel sidecar runs next to the `zosmf-mcp` container and opens an outbound SSH
connection to the jump host, exposing a local SOCKS5 endpoint (`ssh -D`) that the
zosmf-mcp container routes its z/OSMF traffic through. Nothing needs to be installed on
the jump host beyond `sshd` — `ssh -D` is entirely handled by the client side.

The TLS connection to z/OSMF stays end-to-end: it terminates inside the zosmf-mcp
container, not on the jump host. The jump host only ever sees encrypted bytes, so there
are no certificate changes to make and `skip_tls_verify` remains unnecessary.

ZOAU tools do **not** use the SOCKS tunnel above — they open their own, independent
outbound SSH connection straight to the target LPAR, using `ssh -J` (`ProxyJump`) to
reach it via the jump host:

```text
MacBook (privat)          Homelab-Dockerhost           Maschine 2                z/OS
──────────────────        ────────────────────         ──────────────            ──────
Claude Code / Agent  ──▶  zosmf-mcp        ──────────▶ sshd (Jump)   ──▶  sshd (LPAR, ZOAU)
                 (MCP über HTTP)    (SSH, -J via jump host)
```

Because the two paths are independent, the jump host has to be configured in two
places: `JUMP_HOST` in `.env` for the tunnel sidecar (z/OSMF REST), and
`ssh_proxy_jump.host` in the config file for ZOAU. They are typically the same host but
nothing enforces that.

### Config schema

Two independent, optional top-level blocks control forwarding. Neither block, or
`enabled: false`, means today's behavior — requests go out directly.

```yaml
# Route all z/OSMF REST calls through a proxy (e.g. a SOCKS5 tunnel to a jump host).
# Without this block, all requests go out directly.
# proxy:
#   enabled: true
#   url: ${ZOSMF_PROXY}          # socks5://tunnel:1080

# Route ZOAU SSH connections through a jump host (ssh -J).
# ssh_proxy_jump:
#   enabled: true
#   host: jump@jumphost.example.com

# Exempt an individual system from the proxy:
# servers:
#   - name: SYS2
#     host: https://zosmf-internal.example.com:443
#     proxy:
#       enabled: false
```

| Field                  | Scope                | Description                                                              |
|------------------------|-----------------------|----------------------------------------------------------------------------|
| `proxy.enabled`        | global                | Turns the global z/OSMF REST proxy on                                     |
| `proxy.url`            | global                | Proxy URL — scheme must be `http`, `https` or `socks5`                    |
| `servers[].proxy`      | per system            | Overrides the global `proxy` block for one system                         |
| `ssh_proxy_jump.enabled` | global              | Turns the global ZOAU SSH jump host on                                    |
| `ssh_proxy_jump.host`  | global                | Jump host as `user@host`, passed to `ssh -J`                              |
| `ssh_systems[].proxy_jump` | per SSH system   | Overrides the global `ssh_proxy_jump` block for one SSH system            |

Precedence: a per-system (or per-SSH-system) `proxy` / `proxy_jump` block always wins
over the global block, whether it turns forwarding on or off for that one entry. If a
per-system block omits `url` (or a per-SSH-system block omits `host`), it inherits the
value from the global block — so `proxy: {enabled: true}` on one system means "on for
this system, using the global proxy URL." If `proxy.url` is empty at the global level,
it is filled from the `ZOSMF_PROXY` environment variable. An invalid proxy URL scheme is
rejected at config load time, not on the first tool call, and so is `enabled: true`
without a `url` / `host`.

> **Constraint:** `ssh -J` (ProxyJump) propagates the `-i` key given to the target
> connection to the jump-host connection as well, so the jump host and the target z/OS
> LPAR must accept the *same* private key. Inside the container there is no
> `~/.ssh/config`, no agent, and `BatchMode=yes` is set, so there is no way to configure
> a different key per hop — plan the jump host's `authorized_keys` accordingly.

### API-key authentication

The HTTP and Streamable HTTP transports require an API key before they will start:

```bash
make apikey        # openssl rand -base64 32
```

Put the generated value in `.env` next to `docker-compose.yml` (already covered by
`.gitignore`):

```bash
MCP_API_KEY=...
ZOSMF_PASSWORD=...
```

The server reads it as `transport.http.api_key` / `transport.streamable_http.api_key`
(each transport has its own key) and expects clients to send it as
`Authorization: Bearer <key>`. Starting an HTTP transport without an `api_key` set
aborts the server at startup; pass `allow_insecure: true` on that transport to opt into
running without a key.

Client-side, add the header to the MCP client configuration, e.g. `.mcp.json`:

```json
{
  "mcpServers": {
    "zosmf": {
      "type": "http",
      "url": "http://dockerhost:8081/mcp",
      "headers": { "Authorization": "Bearer ..." }
    }
  }
}
```

> **Warning:** Over plain HTTP, the API key only protects against a device that
> stumbles onto or scans the port. It does **not** protect against anyone able to read
> the traffic — the key and every z/OS command travel in the clear. TLS, via a reverse
> proxy in front of the transport, is the next step if that matters for your network.

### Container operation

**Full step-by-step guide: [`container-setup.md`](container-setup.md)** — covers
both Docker/Compose and Apple container, with every command, the required `.env`
entries, key permissions, verification and troubleshooting.

The short version:

```bash
cp .env.example .env     # fill in JUMP_HOST, MCP_API_KEY, per-system passwords, mount paths
make apikey              # generate the bearer token
make docker-up           # Docker:          builds and starts tunnel + zosmf-mcp
make container-up        # Apple container: same, resolving the tunnel IP at runtime
```

The container mounts your existing configuration — it does not ship one. Three things
are worth knowing before the first start:

- **The transport is overridden on the command line** (`--transport streamable-http`).
  A production config usually selects stdio, and nothing is attached to stdin inside a
  container, so the server would exit immediately and restart in a loop.
- **The proxy is switched on by `ZOSMF_PROXY`** — no `proxy:` block needed in the
  mounted file. A block in the file still wins.
- **Unresolved `${VAR}` references abort the start** and are reported by name, instead
  of surfacing later as an unexplained 401.

> **Required on Linux — key ownership:** a bind-mounted SSH key keeps the host's file
> ownership. The tunnel container runs as root, and OpenSSH refuses a key owned by
> anyone else; under `BatchMode=yes` that failure is silent. Before the first start:
>
> ```bash
> sudo chown 0:0 secrets/jump_key && sudo chmod 600 secrets/jump_key
> ```
>
> Not needed on macOS, where the filesystem layer remaps ownership.


## Additional Notes

- **TLS**: For production systems, set `skip_tls_verify: false` and configure CA certificates correctly.
- **Permissions**: The configured z/OSMF user requires appropriate SAF/RACF permissions for the z/OSMF REST APIs being used.
- **TSO**: `tso_account` and `tso_proc` must be set in the configuration to use `zosmf_tso_command`. The logon procedure must be valid on the target system (e.g. `IZUFPROC`).
- **SMP/E**: If `csi_dataset` and `zone` are not specified explicitly, the default values from the system configuration are used.
- **Passwords**: Never hardcode passwords in the configuration file. Always use environment variables (`${VAR}`).

