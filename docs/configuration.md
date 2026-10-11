# Configuration

[← README](../README.md)

zosmf-mcp reads one YAML file. You rarely edit it by hand: `zosmf-mcp init config` writes it, and `zosmf-mcp init env` stores the passwords next to it. This page explains those commands first, then every key the file can hold.

- [Setup commands](#setup-commands)
- [Where the file lives](#where-the-file-lives)
- [Example](#example)
- [Systems](#systems)
- [Passwords](#passwords)
- [ZOAU over SSH](#zoau-over-ssh)
- [Transports and API key](#transports-and-api-key)
- [Forwarding through a proxy or jump host](#forwarding-through-a-proxy-or-jump-host)
- [Resources](#resources)
- [Backups and restore](#backups-and-restore)

## Setup commands

| Command | What it does |
| --- | --- |
| `zosmf-mcp init config` | Adds or changes a system. Without field flags, on a terminal, it runs the wizard. |
| `zosmf-mcp init env` | Writes the passwords the configuration needs — and `MCP_API_KEY` for an HTTP transport — to `.env` next to it, mode 600. |
| `zosmf-mcp init restore` | Lists or restores the backups every write leaves behind. |
| `zosmf-mcp init skill` | Installs the setup procedure as a skill for an AI agent: Claude Code, opencode or pi. |

`zosmf-mcp init <command> --help` lists every flag.

### The wizard

`zosmf-mcp init config` on its own backs up the file, lists the systems already in it and offers `[n]` new system, `[1-N]` change a system, `[r]` restore a backup and `[d]` done. It asks for one system in sections — connection, SMP/E, TSO, then optional settings for other tools and for ZOAU — and shows the result as YAML before `Write? [Y/n]`.

- CSIs and zones are entered one per line; an empty line ends the list. The first CSI, and all zones, are what a tool call uses when it names none.
- When changing a system, the current value is shown in brackets and Enter keeps it. A list can be kept, replaced or added to.

### Flags for scripts

`init config` also takes every field as a flag. `--questions` prints the field list as JSON.

| Flag | Purpose |
| --- | --- |
| `--name`, `--host`, `--user` | The required fields. `--host` needs `https://`. |
| `--csi A,B`, `--zones A,B`, `--tso-account`, `--tso-proc`, … | The optional fields, one flag each. |
| `--ssh-host`, `--ssh-user`, `--ssh-key-file`, `--zoau-path`, `--ssh-port` | The ZOAU group: once one of them is given, all but `--ssh-port` are required. |
| `--file` | The configuration file. Default: the file the server would find, else `~/.config/smpe_ls/.smpe-zosmf.yaml`. |
| `--show` | Prints the systems already in the file as JSON, without secrets. |
| `--print` | Shows the system as it would be written; writes nothing. |
| `--check` | Validates the file and reports where each system's password comes from. |

`init env` takes `--file`, `--env-file`, `--questions` (the variables still missing) and `--generate-api-key`. It never prints a value it writes.

## Where the file lives

The server uses the first of these that exists:

1. the path given with `--config`
2. `.smpe-zosmf.yaml` in the current directory
3. `~/.config/smpe_ls/.smpe-zosmf.yaml`
4. `.zosmf-mcp-config.yaml` in the current directory — deprecated

The same file is read by vscode-smpe, vscpdf and nvpdf.nvim. Each tool ignores the keys it does not know.

## Example

```yaml
# ~/.config/smpe_ls/.smpe-zosmf.yaml
servers:
  - name: PROD
    host: https://zosmf.example.com:443
    user: USER1
    rejectUnauthorized: true
    csi:
      - SMPE.GLOBAL.CSI
    zones: [TGT1]
    tso_account: ACCT1
    tso_proc: IZUFPROC

# ssh_systems:                  # only for the ZOAU tools
#   - name: PROD
#     host: prod.example.com
#     username: user1
#     key_file: ~/.ssh/id_ed25519
#     environment:
#       ZOAU_HOME: /usr/lpp/IBM/zoautil
#       PATH: /usr/lpp/IBM/zoautil/bin:/bin:/usr/bin
#       LIBPATH: /usr/lpp/IBM/zoautil/lib:/lib:/usr/lib

transport:
  stdio: true
```

## Systems

`servers:` — or `systems:` — is a list. Each entry is one z/OSMF endpoint.

| Key | Required | Meaning |
| --- | --- | --- |
| `name` | yes | The name tool calls use as `system`. Upper and lower case do not matter. |
| `host` | yes | The z/OSMF base URL, with `https://` (or `http://`), optionally with a port. |
| `port` | no | Appended to `host` when the URL carries no port. |
| `user` | for password login | The z/OSMF user ID. |
| `auth.password` | no | The password, normally as a `${VAR}` reference. Leave it out to use `ZOSMF_<SYSTEM>_PASSWORD` — see [Passwords](#passwords). |
| `auth.type` | no | `basic` — the default once a user is set — or `certificate`. |
| `auth.cert_file`, `auth.key_file` | for `certificate` | PEM client certificate and its key. |
| `rejectUnauthorized` | no | `false` turns off the TLS certificate check. Only for test systems. |
| `ca_file` | no | PEM CA certificate for a self-signed z/OSMF certificate — the alternative to turning the check off. |
| `csi` | no | CSI data sets. SMP/E queries use the first one when they name none. |
| `zones` | no | SMP/E zones that a query uses when it names none. |
| `tso_account`, `tso_proc` | for `zosmf_tso_command` | Without them the tool refuses to run. A typical procedure is `IZUFPROC`. |
| `proxy` | no | Overrides the global `proxy` block for this system — see [Forwarding](#forwarding-through-a-proxy-or-jump-host). |
| `defaultZones`, `home` | no | Read by vscode-smpe, vscpdf and nvpdf; ignored by zosmf-mcp. |
| `tso_charset`, `tso_codepage`, `dlib_zones` | no | Accepted, but not used. |

Several keys have a second spelling, and both work: `systems` / `servers`, `auth.username` / `user`, `skip_tls_verify: true` / `rejectUnauthorized: false`, `csi_datasets` / `csi`, `target_zones` / `zones`. `zosmf-mcp init config` writes the second form, the one the other tools use, and keeps `systems:` when the file already has it.

A system without any credentials still loads; only its tool calls fail. The configured user needs SAF/RACF access to the z/OSMF REST services the tools use — and no more, see [Before You Start](../README.md#️-before-you-start).

## Passwords

Never put a password in the configuration file. A system's password is looked up in this order:

1. `auth.password` in the file — normally a reference such as `${ZOSMF_SYSA_PASSWORD}`;
2. `ZOSMF_<SYSTEM>_PASSWORD`, derived from the system's `name`: uppercased, every character other than A-Z and 0-9 replaced by `_`. Nothing has to be added to the file for this one;
3. `ZOSMF_PASSWORD`, one password for every system.

Each variable is taken from the process environment first, then from the `.env` file next to the configuration. `zosmf-mcp init env` writes that file (for example `~/.config/smpe_ls/.env`), mode 600. **The server reads it itself at startup** — nothing to source, nothing to pass to an MCP client. A `.env` that group or others can read is ignored, with a startup warning naming the `chmod 600` that fixes it. `KEY=value` lines, `export ` prefixes and quoted values are understood. By hand:

```bash
# ~/.config/smpe_ls/.env — chmod 600, never commit it
ZOSMF_SYSA_PASSWORD=my_password
```

A `${VAR}` reference that resolves nowhere stops the server at startup and is named in the error, instead of reaching z/OSMF as literal text.

The startup log and `zosmf-mcp init config --check` name the source each system used — `file`, `system-env`, `global-env`, `env-file` or `none` — never the value itself.

## ZOAU over SSH

The `zoau_*` tools run ZOAU commands over SSH, not through z/OSMF. They are registered only when `ssh_systems` has an entry. One z/OSMF endpoint can serve a whole sysplex; SSH needs one entry per LPAR. Tool calls name the entry as `ssh_system` — a separate namespace from the `system` of the `zosmf_*` tools.

```yaml
ssh_systems:
  - name: MVSA
    host: mvsa.example.com
    username: ibmuser
    key_file: ~/.ssh/id_ed25519
    environment:
      ZOAU_HOME: /usr/lpp/IBM/zoautil
      PATH: /usr/lpp/IBM/zoautil/bin:/bin:/usr/bin
      LIBPATH: /usr/lpp/IBM/zoautil/lib:/lib:/usr/lib
```

| Key | Required | Meaning |
| --- | --- | --- |
| `name` | yes | The name tool calls use as `ssh_system`. |
| `host` | yes | SSH host name or IP address. |
| `port` | no | Default `22`. |
| `username` | yes | The z/OS user the commands run as. |
| `key_file` | no | The private key. Without it, `ssh` uses its default key. Only key authentication is supported; `init config` always asks for a key. |
| `environment` | no | Variables exported before every command. ZOAU needs `ZOAU_HOME` and its `bin` and `lib` directories in `PATH` and `LIBPATH`, unless the user's login already sets them. `init config --zoau-path` writes all three. |
| `proxy_jump` | no | Overrides the global `ssh_proxy_jump` block for this entry — see [Forwarding](#forwarding-through-a-proxy-or-jump-host). |

The key grants shell access to z/OS; protect it like one.

## Transports and API key

```yaml
transport:
  stdio: true               # the client starts the server: Claude Code, Claude Desktop, IDEs
  streamable_http:          # the server runs on its own: n8n, containers
    enabled: true
    listen: ":8081"
  http:                     # older plain JSON-RPC over HTTP
    enabled: false
    listen: ":8080"
```

| Key | Meaning |
| --- | --- |
| `stdio` | Serves one client over stdin and stdout. |
| `streamable_http.enabled`, `http.enabled` | Starts that HTTP transport. |
| `listen` | Address to listen on. Defaults: `:8081` and `:8080` — all interfaces. `127.0.0.1:8081` keeps it on this machine. |
| `api_key` | The bearer token clients must send. Defaults to `MCP_API_KEY`. |
| `allow_insecure` | `true` starts the transport without an API key. Only when nobody else can reach the port. |

All enabled transports run at the same time. `--transport stdio|http|streamable-http` overrides the file for one run; the default, `auto`, follows it. `zosmf-mcp init config` adds `transport: {stdio: true}` to a file that has no transport block.

An HTTP transport refuses to start without an API key. Generate one:

```bash
zosmf-mcp init env --generate-api-key
```

It writes `MCP_API_KEY` to the `.env` next to the configuration; the server reads it at startup and uses it for both HTTP transports, unless the file sets its own `api_key`. The key is never printed. Clients send it as `Authorization: Bearer <key>`, for example in a `.mcp.json`:

```json
{
  "mcpServers": {
    "zosmf-mcp": {
      "type": "http",
      "url": "http://zosmf-mcp.example.com:8081/mcp",
      "headers": { "Authorization": "Bearer ..." }
    }
  }
}
```

> **Warning:** The HTTP transports carry no TLS. Over plain HTTP the API key only keeps out someone who stumbles onto the port — anyone who can read the traffic sees the key and every z/OS command. Put a TLS-terminating reverse proxy in front if that matters on your network.

## Forwarding through a proxy or jump host

For a server that cannot reach z/OS directly, two optional blocks route its traffic through a machine that can. [Running zosmf-mcp in a container](container-setup.md) shows the complete setup with a tunnel container.

```yaml
proxy:                          # z/OSMF REST calls, e.g. through a SOCKS5 tunnel
  enabled: true
  url: socks5://tunnel:1080
ssh_proxy_jump:                 # ZOAU SSH connections, via ssh -J
  enabled: true
  host: jump@jumphost.example.com
```

| Key | Scope | Meaning |
| --- | --- | --- |
| `proxy.enabled` | global | Turns the z/OSMF proxy on. |
| `proxy.url` | global | Proxy URL; the scheme must be `http`, `https` or `socks5`. Empty: taken from `ZOSMF_PROXY`. |
| `servers[].proxy` | per system | Overrides the global block for one system. |
| `ssh_proxy_jump.enabled` | global | Turns the ZOAU jump host on. |
| `ssh_proxy_jump.host` | global | Jump host as `user@host`, passed to `ssh -J`. |
| `ssh_systems[].proxy_jump` | per SSH system | Overrides the global block for one SSH system. |

Without either block — or with `enabled: false` — requests go out directly. Setting `ZOSMF_PROXY` on its own also turns the proxy on.

A per-system block always wins over the global one, whether it turns forwarding on or off. A per-system block without `url` (or `host`) inherits it from the global block, so `proxy: {enabled: true}` on one system means "on, with the global URL". An invalid scheme, or `enabled: true` without a URL or host, is rejected when the file is loaded, not on the first tool call.

> **The jump host and the LPAR must accept the same key.** zosmf-mcp passes the entry's `key_file` to both hops. In a container there is also no `~/.ssh/config` or agent that could offer a different one.

## Resources

```yaml
resources:
  inline_max_bytes: 32768
```

A read result larger than this — 32 KB by default — comes back as a preview plus a link to the full content. See [Resources](tools.md#resources).

## Backups and restore

`zosmf-mcp init config` and `zosmf-mcp init env` copy the file they are about to replace into `.zosmf-mcp-backups/` next to it — for example `~/.config/smpe_ls/.zosmf-mcp-backups/.smpe-zosmf.yaml.20261001-140530`. The newest 10 copies per file are kept, all mode 600. The wizard also backs up the file when it starts.

To put one back, use `[r]` in the wizard, or:

```bash
zosmf-mcp init restore --list                       # JSON, newest first
zosmf-mcp init restore --backup 20261001-140530     # add --target config|env if both match
```

A restore replaces the whole file and backs up the version it replaces first, so it can itself be undone. `--file` and `--env-file` select the files as in `init config` and `init env`; without `--env-file` it is the `.env` next to the configuration.
