# Security Policy

## Reporting a vulnerability

Report security issues through GitHub's private vulnerability reporting:
**Security → Report a vulnerability** on
<https://github.com/cybersorcerer/zosmf-mcp/security/advisories/new>.

Please do not open a public issue for a security problem, and do not include
credentials, host names, dataset names or captured traffic from a real system
in the report — a minimal reproduction against a test LPAR is enough.

Expect a first response within 14 days. If a fix is warranted, it ships in the
next patch release and the advisory is published once that release is out.

## Supported versions

Only the latest release receives fixes. There are no maintained release
branches.

## What this server is

`zosmf-mcp` gives an MCP client — typically an AI assistant — the authority of
the z/OSMF user and the SSH user it is configured with. That includes tools
that delete datasets, cancel jobs, unmount filesystems and run arbitrary
commands on USS. This is the intended design, documented in the README under
"Before You Start". Reports that amount to "the AI could delete a dataset" are
not vulnerabilities; reports that the server exceeds the configured user's
authority, leaks credentials, or lets an unauthenticated caller reach the
z/OS system are.

## Deployment expectations

The security of an installation depends on choices the operator makes. The
following are documented behaviors, not defects:

- **The HTTP and Streamable HTTP transports carry no TLS.** The bearer token
  and every command travel in the clear. Terminate TLS in a reverse proxy, or
  keep the port on a trusted network.
- **Both HTTP transports bind all interfaces by default** (`:8080` / `:8081`).
  Set `listen` to `127.0.0.1:<port>` when only local clients need access.
- **An HTTP transport refuses to start without an `api_key`** unless
  `allow_insecure: true` is set explicitly. Do not set it on a reachable port.
- **`skip_tls_verify: true` disables certificate validation** towards z/OSMF.
  It exists for self-signed test systems.
- **Passwords belong in environment variables**, not in the configuration
  file. Unresolved `${VAR}` references abort startup rather than reaching
  z/OSMF as literal text.

## Handling of secrets

The server never logs credentials, bearer tokens or `Authorization` headers.
Rejected requests are logged with the reason only — whether a token was
present, never its value. Tool results carry whatever z/OSMF returns, so treat
transcripts from a real system as sensitive.
