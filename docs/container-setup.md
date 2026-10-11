# Running zosmf-mcp in a container

Two situations, and they need different setups:

- **The host reaches z/OSMF itself.** One container, nothing else. Skip to
  [section 9](#9-direct-access-no-jump-host) — the tunnel described below
  does not apply to you.
- **The host cannot reach z/OSMF** and only gets there through an
  intermediate host you can SSH into. That is what sections 1 to 8 cover.

Both work with **Docker/Podman and Compose** (Linux, Windows, macOS) and with
**Apple container** (macOS on Apple Silicon). Every path here was exercised
against a live z/OSMF.

---

## 1. Topology

```text
Agent (n8n, Claude Code, …)
      │  HTTP + bearer token
      ▼
┌─────────────────────────────────────────┐
│ Container host                          │
│                                         │
│  ┌───────────┐        ┌───────────────┐ │
│  │ zosmf-mcp │───────▶│ tunnel        │ │
│  │  :8081    │ SOCKS5 │  ssh -N -D    │ │
│  └───────────┘  :1080 └───────┬───────┘ │
└─────────────────────────────────┼───────┘
                                  │ SSH
                                  ▼
                          ┌───────────────┐
                          │ Jump host     │  sshd only,
                          │ (sshd)        │  nothing to install
                          └───────┬───────┘
                                  │
                                  ▼
                          z/OSMF (HTTPS :443)
```

Two containers. The `tunnel` container opens a SOCKS5 proxy with `ssh -N -D`
and keeps it up. The `zosmf-mcp` container sends its z/OSMF REST calls through
that proxy. Nothing is installed on the jump host — an sshd and an authorized
key are enough.

The SOCKS port 1080 is **never** published. Only the MCP server's port 8081 is
reachable, and it requires a bearer token.

TLS stays end to end: SOCKS5 tunnels the TCP connection, and the TLS handshake
happens between the container and z/OSMF. The jump host only ever sees
encrypted traffic.

The ZOAU tools take a different path. They do not use the tunnel; each call
opens its own SSH connection from the `zosmf-mcp` container to the LPAR, via
the jump host (`ssh -J`):

```text
zosmf-mcp ──SSH──▶ Jump host ──SSH──▶ LPAR (sshd, ZOAU)
```

So the jump host is named in two places: `JUMP_HOST` in `.env` for the tunnel,
and `ssh_proxy_jump.host` in the configuration file for ZOAU — see
[Forwarding through a proxy or jump host](configuration.md#forwarding-through-a-proxy-or-jump-host).
They are usually the same host, but nothing enforces that. For ZOAU, the jump
host must accept the same key as the LPAR — the tunnel uses its own `JUMP_KEY`.

---

## 2. Prerequisites

**On the jump host:** a running sshd and an authorized public key. Nothing else.

**On the container host:** a clone of this repository — the image is built
from it, there is no published image — and Docker with the Compose plugin, or Apple container
1.2+ on macOS 26 (Apple Silicon). Exercised with Apple container 1.2 on
macOS 26 and 1.4.1 on macOS 27; the latter needs the kernel step in
[section 7](#7-path-b-apple-container).

**Network:** before anything else, confirm the jump host can reach z/OSMF:

```bash
ssh <user>@<jump-host> 'nc -z -w 3 <zosmf-host> 443 && echo OK || echo FAIL'
```

If that does not print `OK`, the rest of this guide will not help.

---

## 3. The configuration file

The container mounts your **existing** configuration. There is no separate
container config, and you do not have to rewrite your production file.

Three things matter:

**The transport is overridden at startup.** Your file most likely says
`transport: stdio: true`. Inside a container nothing is attached to stdin — the
server would hit EOF immediately and end up in a restart loop. So the transport
is forced on the command line; the Compose file and the make targets already do
this:

```
--transport streamable-http
```

**The proxy is switched on through the environment.** You do not need a
`proxy:` block in the file. Setting `ZOSMF_PROXY` supplies the URL and enables
the proxy. A `proxy:` block in the file takes precedence, and a
`proxy: {enabled: false}` on an individual system remains an effective opt-out.

**Passwords** come from the container's environment: `ZOSMF_<SYSTEM>_PASSWORD`
for each system — derived from its name, nothing to add to the file — and any
`${...}` reference the file contains. If a referenced variable is not set, the
server **refuses to start** and names the missing variables. That is deliberate —
previously the placeholder survived as literal text and surfaced much later as
an unexplained 401.

---

## 4. Create `.env`

```bash
cp .env.example .env
```

Fill it in. The file is in `.gitignore`.

```bash
JUMP_HOST=jumpuser@jumphost.example.com
JUMP_KEY=./secrets/jump_key
ZOSMF_CONFIG=/home/youruser/.config/smpe_ls/.smpe-zosmf.yaml
SSH_KEY_DIR=./secrets
MCP_API_KEY=<output of: make apikey>
ZOSMF_SYSA_PASSWORD=…
ZOSMF_SYSB_PASSWORD=…
```

Name each password variable `ZOSMF_<SYSTEM>_PASSWORD` after a system in your
configuration (uppercase; any character other than A-Z and 0-9 becomes `_`), or
exactly like a `${...}` reference the file contains.

> **Plain `KEY=value` only — no leading `export`.** Both `docker compose`
> (`env_file`) and `container run --env-file` reject the export form. Apple
> container reports:
> `Error: variable 'export ZOSMF_SYSA_PASSWORD' contains whitespaces`

Generate the API key:

```bash
make apikey          # openssl rand -base64 32
```

---

## 5. SSH keys and where they live

### Two different things

`JUMP_KEY` and `SSH_KEY_DIR` are easy to mix up. They serve different
containers and only one of them is a directory:

| Variable | What it points at | Default | Inside the container | Needed for |
| --- | --- | --- | --- | --- |
| `JUMP_KEY` | a single **file** — the key for the tunnel hop | `./secrets/jump_key` | `/key`, in the tunnel container | the tunnel setup only |
| `SSH_KEY_DIR` | a **directory** holding the key(s) for the ZOAU tools | `./secrets` | `/keys`, in the server container | the `zoau_*` tools only |

Both may point at the same key if it is authorized on both hops.

Relative paths are resolved against the directory you run `make` from, so the
defaults land in the repository. Absolute paths work the same way, with both
Docker and Apple container. The directory is outside the image: `secrets/` is
in `.gitignore` and `.dockerignore`, so keys are never committed or baked in.

With `make container-up-direct` the key directory is **optional** — it is
mounted only if it exists, and a host that runs no ZOAU tools needs none.

### The configuration file names the container's path

`ssh_systems[].key_file` is read inside the container, so it must be the
mounted path, not the one on your host:

```yaml
ssh_systems:
  - name: "SYSA"
    host: "zos1.example.com"
    username: "ibmuser"
    key_file: "/keys/id_rsa"     # not the path it has on your host
```

A host path here fails with `Permission denied` and no further explanation,
because the file simply is not there.

### Create the tunnel key

Create a **dedicated** key for the tunnel. Do not reuse your everyday personal
key — the container key sits unencrypted on the host and should be authorized
for this one jump host only.

```bash
mkdir -p secrets
ssh-keygen -t ed25519 -N '' -f secrets/jump_key -C 'zosmf-mcp tunnel'
ssh-copy-id -i secrets/jump_key.pub <user>@<jump-host>
```

**Additionally required on Linux:** the tunnel container runs as root and sees
the real host UID of the mounted file. If it is owned by your user, OpenSSH
refuses it and the container restarts in a loop.

```bash
sudo chown 0:0 secrets/jump_key
sudo chmod 600 secrets/jump_key
```

Not needed on macOS, where the filesystem layer remaps ownership.

---

## 6. Path A: Docker with Compose

### Build and start

```bash
make docker-build     # docker compose build
make docker-up        # docker compose --profile tunnel up -d
make docker-logs      # docker compose logs -f
make docker-down      # docker compose --profile tunnel down
```

Or without make:

```bash
docker compose build
docker compose --profile tunnel up -d
docker compose logs -f zosmf-mcp
```

### What the Compose file does

```yaml
services:
  tunnel:
    profiles: ["tunnel"]
    image: alpine/openssh
    user: "0:0"
    entrypoint: ["/usr/bin/ssh"]
    command: >
      -N -D 0.0.0.0:1080 -i /key
      -o ExitOnForwardFailure=yes
      -o ServerAliveInterval=30
      -o ServerAliveCountMax=3
      -o StrictHostKeyChecking=accept-new
      -o BatchMode=yes
      ${JUMP_HOST}
    volumes:
      - ${JUMP_KEY:-./secrets/jump_key}:/key:ro
    restart: unless-stopped

  zosmf-mcp:
    build:
      context: .
      args:
        VERSION: docker
    depends_on:
      tunnel:
        condition: service_started
        required: false
    command: ["--transport", "streamable-http"]
    ports:
      - "8081:8081"
    volumes:
      - ${ZOSMF_CONFIG:-./.smpe-zosmf.yaml}:/config/.smpe-zosmf.yaml:ro
      - ${SSH_KEY_DIR:-./secrets}:/keys:ro
    env_file:
      - .env
    environment:
      ZOSMF_PROXY: ${ZOSMF_PROXY-socks5://tunnel:1080}
    restart: unless-stopped
```

The service name `tunnel` resolves via DNS inside the Compose network, which is
why it appears directly in `ZOSMF_PROXY`. `env_file` passes **every** variable
through, including the per-system passwords.

The tunnel sits behind a **profile**, so `docker compose up` alone starts the
server by itself — see [section 9](#9-direct-access-no-jump-host). `make
docker-up` adds `--profile tunnel`, and `required: false` on the dependency is
what lets the server start when the profile is off.

`depends_on` only waits for the tunnel container to *start*, not for the SSH
connection to come up. The tunnel is typically ready in well under a second, so
an agent connecting later never notices.

---

## 7. Path B: Apple container

Apple container has no Compose equivalent. The two containers are started
individually, and there is **no name resolution between containers** without a
registered DNS domain — so the tunnel's IP is resolved at runtime.

### One-time: install the default kernel

A fresh Apple container install has no kernel configured and asks for one the
first time a container starts. Over SSH or from a script nobody answers that
prompt, and the start fails with:

```
Install the recommended default kernel from […kata-static…tar.zst]? [Y/n]:
No default kernel configured.
Please use the `container system kernel set --recommended` command
```

Do it once, up front:

```bash
container system kernel set --recommended
container system status            # status running
```

### One-time: drop the Rosetta requirement

Apple's BuildKit builder runs in a VM that by default wires in Rosetta so it can
also build `linux/amd64`. Without Rosetta the builder fails to start at all:

```
Error: failed to bootstrap container buildkit … "Rosetta is not installed"
```

Our image is pure arm64. Rather than installing Rosetta, turn the requirement
off:

```bash
mkdir -p ~/.config/container
cat > ~/.config/container/config.toml <<'EOF'
[build]
rosetta = false
EOF

container system stop
container system start
container system property list | head -6   # rosetta = false
```

The file is merged with the defaults; all other settings stay as they were.

### Build and start

```bash
make container-build   # container build --platform linux/arm64 -t zosmf-mcp:latest .
make container-up      # tunnel + server, including IP resolution
make container-logs    # container logs -f zosmf-mcp
make container-down    # stop and remove what is running
```

`container-down` serves both setups: it stops the server and the tunnel
individually, so it stays quiet on a host that never had a tunnel.

### The same steps without make

```bash
# 1) Start the builder and build the image
container builder start
container build --platform linux/arm64 -t zosmf-mcp:latest .

# 2) Start the tunnel
container run -d --name zosmf-tunnel \
  -v "$PWD/secrets/jump_key:/key" \
  --entrypoint /usr/bin/ssh zosmf-mcp:latest \
  -N -D 0.0.0.0:1080 -i /key \
  -o ExitOnForwardFailure=yes -o ServerAliveInterval=30 \
  -o ServerAliveCountMax=3 -o StrictHostKeyChecking=accept-new \
  -o BatchMode=yes jumpuser@jumphost.example.com

# 3) Resolve the tunnel's IP
TUNNEL_IP=$(container ls | awk '/^zosmf-tunnel/{print $6}' | cut -d/ -f1)
echo "$TUNNEL_IP"

# 4) Normalise .env for --env-file (strips a leading "export" if present)
sed -E 's/^[[:space:]]*export[[:space:]]+//' .env \
  | grep -E '^[A-Za-z_][A-Za-z0-9_]*=' > /tmp/zosmf-env
chmod 600 /tmp/zosmf-env

# 5) Start the MCP server
container run -d --name zosmf-mcp \
  -v "$HOME/.config/smpe_ls/.smpe-zosmf.yaml:/config/.smpe-zosmf.yaml" \
  -v "$PWD/secrets:/keys" \
  --env-file /tmp/zosmf-env \
  -e ZOSMF_PROXY=socks5://$TUNNEL_IP:1080 \
  -p 8081:8081 \
  zosmf-mcp:latest --transport streamable-http

rm -f /tmp/zosmf-env
```

Using our own image for the tunnel has a reason: Apple container can fail to
pull `alpine/openssh` anonymously from Docker Hub (`401 Unauthorized`), and our
image already contains `openssh-client`. Under Docker, `alpine/openssh` works
normally.

---

## 8. Verifying the setup

```bash
# Are the containers running?
docker compose ps            # or: container ls

# Is the tunnel up?
docker compose logs tunnel   # or: container logs zosmf-tunnel
# Expected: no errors. "Permanently added … to the list of known hosts"
# on the first start is normal.

# Is the server listening and demanding authentication?
curl -s -o /dev/null -w '%{http_code}\n' -X POST http://localhost:8081/mcp \
  -H 'Content-Type: application/json' -d '{}'
# Expected: 401
```

> This call sends **no** token on purpose, so `401` is the result that proves
> the server is protected. Without `-o /dev/null -w '%{http_code}'` the same
> answer prints as its body text, `unauthorized` — that is a pass, not a
> failure. The server's log says which of the two it was:
> `rejected POST /mcp: no bearer token` means no `Authorization` header
> arrived at all, `wrong bearer token` means the key did not match.
> Container logs are in UTC, which is worth remembering when matching them
> against the time on your machine.

Full round trip with a real tool call:

```bash
KEY=<your MCP_API_KEY>

SID=$(curl -s -D - -o /dev/null -X POST http://localhost:8081/mcp \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}' \
  | tr -d '\r' | awk -F': ' '/[Mm]cp-[Ss]ession-[Ii]d/{print $2}')

curl -s -X POST http://localhost:8081/mcp -H "Authorization: Bearer $KEY" \
  -H "Mcp-Session-Id: $SID" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}' >/dev/null

curl -s -X POST http://localhost:8081/mcp -H "Authorization: Bearer $KEY" \
  -H "Mcp-Session-Id: $SID" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"zosmf_list_datasets","arguments":{"system":"SYSA","dslevel":"SYS1.PARM*"}}}'
```

If that returns a dataset list, the whole chain works: HTTP auth, proxy,
tunnel, jump host, z/OSMF.

---

## 9. Direct access (no jump host)

When the container host can reach z/OSMF itself, there is nothing to tunnel.
One container, no SOCKS proxy, and the same image as above.

### `.env`

Only three things matter. `JUMP_HOST`, `JUMP_KEY` and `ZOSMF_PROXY` are not
used — leave them out, or set `ZOSMF_PROXY=` empty if the file is shared with
a tunnelled setup elsewhere.

```bash
ZOSMF_CONFIG=/home/youruser/.config/smpe_ls/.smpe-zosmf.yaml
MCP_API_KEY=<output of: make apikey>
ZOSMF_SYSA_PASSWORD=…
```

`SSH_KEY_DIR` is only needed for the ZOAU tools. The directory is mounted at
`/keys` when it exists and skipped when it does not, so a host that runs no
ZOAU tools needs no keys at all — see [section 5](#5-ssh-keys-and-where-they-live)
for how the paths map, and remember that `key_file` in the configuration is
the path *inside* the container.

### Start it

```bash
make docker-up-direct      # Docker: compose without the tunnel profile
make container-up-direct   # Apple container: one `container run`
```

Both publish `8081` and force `--transport streamable-http`, because nothing
is attached to stdin inside a container.

Verify as in [section 8](#8-verifying-the-setup) — an unauthenticated POST
must answer `401`, and a tool call must return real data.

### Autostart at login (macOS)

Docker restarts containers on its own through `restart: unless-stopped`.
Apple container has no restart policy, so a LaunchAgent does the job:

```bash
make launch-agent          # install and start it
make launch-agent-remove   # remove it again
```

The agent runs `scripts/container-autostart.sh` as the logged-in user. That
script starts the `container` apiserver first — after a reboot it is neither
running nor registered with launchd — then starts the existing container, or
creates one through `make container-up-direct` if there is none.

```bash
launchctl list | grep zosmf-mcp        # 0 in the second column means it ran
tail -f ~/Library/Logs/zosmf-mcp.log   # what the agent did
launchctl kickstart gui/$(id -u)/com.github.cybersorcerer.zosmf-mcp
```

It runs at **login**, not at boot: a Mac that reboots unattended serves
nothing until someone signs in. Enable automatic login, or keep the machine
logged in, if that matters.

A changed configuration or a new image needs

```bash
make container-down && make container-up-direct
```

— restarting the agent only starts the container that already exists.

## 10. Connecting a client

Endpoint: `http://<container-host>:8081/mcp`
Header: `Authorization: Bearer <MCP_API_KEY>`

For the n8n MCP Client Tool node (v1.88+), enter the URL as the server URL and
the bearer token as header auth.

The token travels over the network in clear text, so this setup belongs in a
trusted network. To expose it more widely, put a TLS-terminating reverse proxy
in front — the server itself does not speak HTTPS.

---

## 11. Troubleshooting

**`unresolved environment variables in …: ZOSMF_SYSA_PASSWORD`**
The configuration references a variable that never reached the container. Check
that the name in `.env` matches exactly and that `env_file` / `--env-file` is
set. On Apple container: does your `.env` still use `export`?

**`transport streamable_http: api_key is required`**
`MCP_API_KEY` is empty. Generate one with `make apikey` and put it in `.env`.
Starting without authentication on purpose requires `allow_insecure: true` in
the `transport` block — only sensible if port 8081 is demonstrably unreachable.

**Tool calls take ~60 seconds and report `context deadline exceeded`**
Traffic is bypassing the tunnel. Check that `ZOSMF_PROXY` is set inside the
container and points at the tunnel. On Apple container: did the tunnel IP change
after a restart? Run `make container-down` then `make container-up`.

**`zoau_*` tools fail with `Permission denied`**
The key is not where the container looks. Either `SSH_KEY_DIR` does not point
at the directory holding it, or `key_file` in the configuration still names a
path on the host instead of `/keys/<name>` — see
[section 5](#5-ssh-keys-and-where-they-live). Check what the container sees:

```bash
container exec zosmf-mcp ls -l /keys     # or: docker compose exec zosmf-mcp ls -l /keys
```

**The tunnel container keeps restarting**
Almost always the key. On Linux it must be owned by root with mode 600.
`docker compose logs tunnel` shows OpenSSH's message.

**`401` from z/OSMF even though the variables are set**
Wrong credentials, or the user is revoked on the target system. Since the
startup check for unresolved variables was added, a missing environment
variable is ruled out — the server would not have started at all.

**Apple container: `HTTP 000` on `localhost:8081` right after startup**
Port publishing takes a moment. Wait a few seconds and retry, or go directly to
the container IP from `container ls`.
