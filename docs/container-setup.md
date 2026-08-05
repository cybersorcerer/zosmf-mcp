# Running zosmf-mcp in a container

How to run zosmf-mcp in a container when the machine hosting it **cannot reach
the z/OSMF systems directly** — only through an intermediate host you can SSH
into.

Two paths are covered: **Docker/Podman with Compose** (Linux, Windows, macOS)
and **Apple container** (macOS on Apple Silicon). Both were exercised against a
live z/OSMF.

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

---

## 2. Prerequisites

**On the jump host:** a running sshd and an authorized public key. Nothing else.

**On the container host:** Docker with the Compose plugin, or Apple container
1.2+ on macOS 26 (Apple Silicon).

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

**Passwords** referenced as `${ZOSMF_SYSA_PASSWORD}` are resolved from the
container's environment. If a referenced variable is not set, the server
**refuses to start** and names the missing variables. That is deliberate —
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

The password variable names must match the `${...}` references in your
configuration file **exactly**.

> **Plain `KEY=value` only — no leading `export`.** Both `docker compose`
> (`env_file`) and `container run --env-file` reject the export form. Apple
> container reports:
> `Error: variable 'export ZOSMF_SYSA_PASSWORD' contains whitespaces`

Generate the API key:

```bash
make apikey          # openssl rand -base64 32
```

---

## 5. Prepare the SSH key

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
make docker-up        # docker compose up -d
make docker-logs      # docker compose logs -f
make docker-down      # docker compose down
```

Or without make:

```bash
docker compose build
docker compose up -d
docker compose logs -f zosmf-mcp
```

### What the Compose file does

```yaml
services:
  tunnel:
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
      - tunnel
    command: ["--transport", "streamable-http"]
    ports:
      - "8081:8081"
    volumes:
      - ${ZOSMF_CONFIG:-./.smpe-zosmf.yaml}:/config/.smpe-zosmf.yaml:ro
      - ${SSH_KEY_DIR:-./secrets}:/keys:ro
    env_file:
      - .env
    environment:
      ZOSMF_PROXY: socks5://tunnel:1080
    restart: unless-stopped
```

The service name `tunnel` resolves via DNS inside the Compose network, which is
why it appears directly in `ZOSMF_PROXY`. `env_file` passes **every** variable
through, including the per-system passwords.

`depends_on` only waits for the tunnel container to *start*, not for the SSH
connection to come up. The tunnel is typically ready in well under a second, so
an agent connecting later never notices.

---

## 7. Path B: Apple container

Apple container has no Compose equivalent. The two containers are started
individually, and there is **no name resolution between containers** without a
registered DNS domain — so the tunnel's IP is resolved at runtime.

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
make container-down    # stop and remove both containers
```

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

Using our own image for the tunnel has a reason: `alpine/openssh` could not be
pulled anonymously from Docker Hub here (`401 Unauthorized`), and our image
already contains `openssh-client`. Under Docker, `alpine/openssh` works
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

## 9. Connecting a client

Endpoint: `http://<container-host>:8081/mcp`
Header: `Authorization: Bearer <MCP_API_KEY>`

For the n8n MCP Client Tool node (v1.88+), enter the URL as the server URL and
the bearer token as header auth.

The token travels over the network in clear text, so this setup belongs in a
trusted network. To expose it more widely, put a TLS-terminating reverse proxy
in front — the server itself does not speak HTTPS.

---

## 10. Troubleshooting

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
