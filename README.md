# zosmf-mcp — User Guide

A [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server for z/OS that gives Claude and other AI assistants direct access to z/OS systems via the z/OSMF REST API.

---

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Installation](#2-installation)
3. [Configuration](#3-configuration)
4. [Registering the MCP Server](#4-registering-the-mcp-server)
   - [Claude Desktop](#41-claude-desktop-macoswindows)
   - [Claude Code CLI](#42-claude-code-cli)
   - [VS Code (Claude Code Extension)](#43-vs-code-claude-code-extension)
   - [JetBrains IDEs](#44-jetbrains-ides)
   - [Gemini CLI](#45-gemini-cli)
   - [Continue.dev](#46-continuedev)
   - [Cursor](#47-cursor)
   - [Anthropic API (custom applications)](#48-anthropic-api-custom-applications)
   - [Direct invocation / custom clients](#49-direct-invocation--custom-clients)
   - [n8n (HTTP transport)](#410-n8n-http-transport)
5. [Tools Overview](#5-tools-overview)
6. [Tool Reference and Example Prompts](#6-tool-reference-and-example-prompts)
7. [Building Automations](#7-building-automations)

---

## 1. Prerequisites

- **Go 1.22+** — to build from source
- **z/OSMF** — enabled on the target system (z/OS V2.3+)
- **Network access** — to the z/OSMF port (default: 443)
- An **MCP-compatible AI client** — Claude Desktop, Claude Code CLI, VS Code, Cursor, etc.

---

## 2. Installation

### Build from Source

```bash
git clone https://github.com/ronaldfunk/zosmf-mcp.git
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

---

## 3. Configuration

Configuration is done via a YAML file. By default the server looks for `.zosmf-mcp-config.yaml` in the working directory.

### Example Configuration

```yaml
systems:
  - name: "SYSA"
    host: "https://sysa.example.com:443"
    auth:
      type: "basic"
      username: "IBMUSER"
      password: "${ZOSMF_SYSA_PASSWORD}"   # From environment variable
    # ca_file: "/path/to/ca.pem"           # Optional: custom CA for self-signed z/OSMF certs
    skip_tls_verify: false
    csi_datasets:                          # Default CSI datasets for SMP/E queries
      - "SMPE.GLOBAL.CSI"
    target_zones:                          # Default SMP/E target zones
      - "TGTZONE"
    dlib_zones:                            # Default SMP/E distribution zones
      - "DLIBZONE"
    tso_account:  "ACCT#"                 # TSO account number
    tso_proc:     "IZUFPROC"              # TSO logon procedure
    tso_charset:  "697"                   # TSO character set (default: 697)
    tso_codepage: "1047"                  # TSO code page (default: 1047)

  # Second system using certificate authentication
  - name: "SYSB"
    host: "https://sysb.example.com:443"
    auth:
      type: "certificate"
      cert_file: "/path/to/client.pem"
      key_file:  "/path/to/client.key"
    ca_file: "/path/to/ca.pem"

transport:
  stdio: true        # For MCP integration (VS Code, Claude Code, etc.)
  http:
    enabled: true    # Optional: HTTP transport for other clients (n8n, curl, etc.)
    listen: ":8080"
```

### Configuration Reference

| Field             | Required | Description                                                       |
|-------------------|----------|-------------------------------------------------------------------|
| `name`            | Yes      | System name used in prompts (e.g. "PROD", "MAINT")               |
| `host`            | Yes      | z/OSMF base URL including port                                    |
| `port`            | No       | Port number — appended to `host` if not already present          |
| `auth.type`       | Yes      | `basic` or `certificate`                                          |
| `auth.username`   | Yes*     | TSO user ID (*required for `basic`)                               |
| `auth.password`   | Yes*     | Password — reference via `${ENV_VAR}` to avoid hardcoding        |
| `auth.cert_file`  | Yes**    | PEM certificate file (**required for `certificate`)               |
| `auth.key_file`   | Yes**    | PEM private key file (**required for `certificate`)               |
| `ca_file`         | No       | PEM file with the CA certificate for self-signed z/OSMF certs    |
| `skip_tls_verify` | No       | Skip TLS validation — not recommended for production             |
| `csi_datasets`    | No       | Default CSI datasets for SMP/E queries (list of dataset names)   |
| `target_zones`    | No       | Default SMP/E target zones                                        |
| `dlib_zones`      | No       | Default SMP/E distribution zones                                  |
| `tso_account`     | No       | TSO account number (required for `zosmf_tso_command`)            |
| `tso_proc`        | No       | TSO logon procedure (required for `zosmf_tso_command`)           |
| `tso_charset`     | No       | TSO character set, default: `697`                                 |
| `tso_codepage`    | No       | TSO code page, default: `1047`                                    |

### Compatibility Aliases

The following fields are accepted as aliases for compatibility with other z/OSMF tools:

| Alias                | Equivalent to                 |
|----------------------|-------------------------------|
| `servers`            | `systems`                     |
| `user`               | `auth.username`               |
| `csi`                | `csi_datasets`                |
| `rejectUnauthorized` | `skip_tls_verify` (inverted)  |

### Passing Passwords Securely

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

If `auth.password` is not set for a system and no `${ENV_VAR}` placeholder is present, the server falls back to the `ZOSMF_PASSWORD` environment variable as a global default.

---

## 4. Registering the MCP Server

zosmf-mcp supports two transports that can run simultaneously:

- **stdio** — the client starts the server process and communicates via stdin/stdout. Used by Claude Desktop, Claude Code, VS Code, etc.
- **HTTP** — the server listens on a TCP port and accepts JSON-RPC 2.0 requests. Used by n8n, curl, and custom clients.

Both transports are controlled via the config file. The `--transport` flag can override this (`stdio`, `http`, or `auto` which reads the config — default is `auto`).

For the stdio clients the configuration is the same everywhere:

- **Command**: path to the `zosmf-mcp` binary
- **Arguments**: `--config /path/to/.zosmf-mcp-config.yaml`
- **Environment variables**: pass passwords as `ENV` variables

---

### 4.1 Claude Desktop (macOS/Windows)

Claude Desktop supports MCP servers via the `claude_desktop_config.json` file.

**Location:**

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "zosmf-mcp": {
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "/Users/<user>/.zosmf-mcp-config.yaml"],
      "env": {
        "ZOSMF_SYSA_PASSWORD": "secret"
      }
    }
  }
}
```

> **Tip:** On macOS, specify the full path to the binary since Claude Desktop does not perform shell PATH expansion. Use `which zosmf-mcp` to find the path.

Restart Claude Desktop after saving. The tools will appear in the chat as available functions (hammer icon).

---

### 4.2 Claude Code CLI

Claude Code (the `claude` CLI tool) reads MCP server configurations from `~/.claude/mcp.json` (global) or `.claude/mcp.json` in the project directory (local).

**Register globally** (applies to all projects):

```bash
claude mcp add zosmf-mcp \
  /Users/<user>/.local/bin/zosmf-mcp \
  -- --config /Users/<user>/.zosmf-mcp-config.yaml
```

Or manually in `~/.claude/mcp.json`:

```json
{
  "mcpServers": {
    "zosmf-mcp": {
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "/Users/<user>/.zosmf-mcp-config.yaml"],
      "env": {
        "ZOSMF_SYSA_PASSWORD": "secret"
      }
    }
  }
}
```

**Local to a project** (`.claude/mcp.json` in the repo):

```json
{
  "mcpServers": {
    "zosmf-mcp": {
      "command": "zosmf-mcp",
      "args": ["--config", ".zosmf-mcp-config.yaml"],
      "env": {
        "ZOSMF_SYSA_PASSWORD": "${ZOSMF_SYSA_PASSWORD}"
      }
    }
  }
}
```

**Check status:**
```bash
claude mcp list
# or in the interactive chat:
/mcp
```

---

### 4.3 VS Code (Claude Code Extension)

MCP servers are registered in VS Code via an `mcp.json` file in the workspace directory.

**`.vscode/mcp.json`** (automatically detected by the extension):

```json
{
  "servers": {
    "zosmf-mcp": {
      "type": "stdio",
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "${workspaceFolder}/.zosmf-mcp-config.yaml"],
      "env": {
        "ZOSMF_SYSA_PASSWORD": "${env:ZOSMF_SYSA_PASSWORD}"
      }
    }
  }
}
```

> `${workspaceFolder}` is resolved by VS Code to the current workspace directory. `${env:VAR}` reads environment variables from the terminal session in which VS Code was launched.

**Check status:** Type `/mcp` in the chat window — all registered servers and their tools will be listed.

---

### 4.4 JetBrains IDEs

JetBrains IDEs (IntelliJ, GoLand, etc.) support MCP via the **AI Assistant plugin**.

**Configuration:** `Settings → Tools → AI Assistant → Model Context Protocol (MCP)`

Add a new server entry:

| Field       | Value                                             |
|-------------|---------------------------------------------------|
| Name        | `zosmf-mcp`                                       |
| Command     | `/Users/<user>/.local/bin/zosmf-mcp`              |
| Arguments   | `--config /Users/<user>/.zosmf-mcp-config.yaml`   |
| Environment | `ZOSMF_SYSA_PASSWORD=secret`                      |

Alternatively, as a JSON configuration in `~/.config/JetBrains/mcp.json` (since 2024.3):

```json
{
  "servers": {
    "zosmf-mcp": {
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "/Users/<user>/.zosmf-mcp-config.yaml"],
      "env": {
        "ZOSMF_SYSA_PASSWORD": "secret"
      }
    }
  }
}
```

---

### 4.5 Gemini CLI

Google Gemini CLI supports MCP servers via a `settings.json` configuration file.

**Location:** `~/.gemini/settings.json`

```json
{
  "mcpServers": {
    "zosmf-mcp": {
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "/Users/<user>/.zosmf-mcp-config.yaml"],
      "env": {
        "ZOSMF_SYSA_PASSWORD": "secret"
      }
    }
  }
}
```

**Check status:** In the Gemini CLI session:
```
/mcp
```

All registered tools become available in the session — the model can use them as native capabilities.

---

### 4.6 Continue.dev

[Continue](https://continue.dev) is an open-source AI coding extension for VS Code and JetBrains. MCP servers are configured in `~/.continue/config.json`:

```json
{
  "experimental": {
    "modelContextProtocolServers": [
      {
        "transport": {
          "type": "stdio",
          "command": "/Users/<user>/.local/bin/zosmf-mcp",
          "args": ["--config", "/Users/<user>/.zosmf-mcp-config.yaml"],
          "env": {
            "ZOSMF_SYSA_PASSWORD": "secret"
          }
        }
      }
    ]
  }
}
```

---

### 4.7 Cursor

[Cursor](https://cursor.sh) supports MCP servers since version 0.43 via `~/.cursor/mcp.json` (global) or `.cursor/mcp.json` in the project directory:

```json
{
  "mcpServers": {
    "zosmf-mcp": {
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "/Users/<user>/.zosmf-mcp-config.yaml"],
      "env": {
        "ZOSMF_SYSA_PASSWORD": "secret"
      }
    }
  }
}
```

After saving, go to `Cursor Settings → MCP` — the server should appear with a green status indicator.

---

### 4.8 Anthropic API (custom applications)

Using the Anthropic API with the **MCP Client SDK**, you can build custom applications that leverage Claude together with zosmf-mcp.

#### Python — with `anthropic` SDK and `mcp` client

```bash
pip install anthropic mcp
```

```python
import asyncio
import anthropic
from mcp import StdioServerParameters
from mcp.client.stdio import stdio_client

SYSTEM = "SYSA"
CONFIG = "/Users/<user>/.zosmf-mcp-config.yaml"

async def main():
    server_params = StdioServerParameters(
        command="/Users/<user>/.local/bin/zosmf-mcp",
        args=["--config", CONFIG],
        env={"ZOSMF_SYSA_PASSWORD": "secret"},
    )

    async with stdio_client(server_params) as (read, write):
        from mcp.client.session import ClientSession
        async with ClientSession(read, write) as session:
            await session.initialize()

            # Load available tools as Anthropic tool definitions
            tools_result = await session.list_tools()
            tools = [
                {
                    "name": t.name,
                    "description": t.description,
                    "input_schema": t.inputSchema,
                }
                for t in tools_result.tools
            ]

            client = anthropic.Anthropic()
            messages = [{"role": "user", "content": f"Show me the current system time on {SYSTEM}"}]

            # Agentic loop: Claude calls tools until the answer is complete
            while True:
                response = client.messages.create(
                    model="claude-opus-4-6",
                    max_tokens=4096,
                    tools=tools,
                    messages=messages,
                )

                if response.stop_reason == "end_turn":
                    for block in response.content:
                        if hasattr(block, "text"):
                            print(block.text)
                    break

                # Execute tool calls
                tool_results = []
                for block in response.content:
                    if block.type == "tool_use":
                        result = await session.call_tool(block.name, block.input)
                        tool_results.append({
                            "type": "tool_result",
                            "tool_use_id": block.id,
                            "content": result.content[0].text if result.content else "",
                        })

                messages.append({"role": "assistant", "content": response.content})
                messages.append({"role": "user", "content": tool_results})

asyncio.run(main())
```

#### TypeScript/Node.js

```bash
npm install @anthropic-ai/sdk @modelcontextprotocol/sdk
```

```typescript
import Anthropic from "@anthropic-ai/sdk";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";

const transport = new StdioClientTransport({
  command: "/Users/<user>/.local/bin/zosmf-mcp",
  args: ["--config", "/Users/<user>/.zosmf-mcp-config.yaml"],
  env: { ...process.env, ZOSMF_SYSA_PASSWORD: "secret" },
});

const mcp = new Client({ name: "my-app", version: "1.0.0" }, { capabilities: {} });
await mcp.connect(transport);

// Load tools
const { tools } = await mcp.listTools();
const anthropicTools = tools.map((t) => ({
  name: t.name,
  description: t.description ?? "",
  input_schema: t.inputSchema,
}));

const client = new Anthropic();
const messages: Anthropic.MessageParam[] = [
  { role: "user", content: "Show me all active jobs on SYSA" },
];

// Agentic loop
while (true) {
  const response = await client.messages.create({
    model: "claude-opus-4-6",
    max_tokens: 4096,
    tools: anthropicTools,
    messages,
  });

  if (response.stop_reason === "end_turn") {
    const text = response.content.find((b) => b.type === "text");
    if (text && text.type === "text") console.log(text.text);
    break;
  }

  const toolResults: Anthropic.ToolResultBlockParam[] = [];
  for (const block of response.content) {
    if (block.type === "tool_use") {
      const result = await mcp.callTool({ name: block.name, arguments: block.input as Record<string, unknown> });
      toolResults.push({
        type: "tool_result",
        tool_use_id: block.id,
        content: (result.content as Array<{ text: string }>)[0]?.text ?? "",
      });
    }
  }

  messages.push({ role: "assistant", content: response.content });
  messages.push({ role: "user", content: toolResults });
}

await mcp.close();
```

> Both examples implement an **agentic loop**: Claude decides which tools to call and continues until it can formulate a complete answer.

---

### 4.9 Direct invocation / custom clients

For custom applications or shell scripts, the server can be started directly and addressed via JSON-RPC 2.0 over stdio:

```bash
# Start the server manually (waits for JSON-RPC on stdin)
ZOSMF_SYSA_PASSWORD=secret zosmf-mcp --config .zosmf-mcp-config.yaml
```

**Example: list available tools**

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' \
  | ZOSMF_SYSA_PASSWORD=secret zosmf-mcp --config .zosmf-mcp-config.yaml
```

**Example: call a tool**

```bash
echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"zosmf_issue_command","arguments":{"system":"SYSA","command":"D T"}}}' \
  | ZOSMF_SYSA_PASSWORD=secret zosmf-mcp --config .zosmf-mcp-config.yaml
```

MCP client SDKs are available for several languages:

- **Python**: `pip install mcp`
- **TypeScript/Node.js**: `npm install @modelcontextprotocol/sdk`
- **Go**: `github.com/mark3labs/mcp-go`

---

### 4.10 n8n (HTTP transport)

[n8n](https://n8n.io) is an open-source workflow automation platform. It can call the zosmf-mcp server via the **HTTP transport** using standard HTTP Request nodes.

#### Enable HTTP transport

In `.zosmf-mcp-config.yaml`:

```yaml
transport:
  stdio: true       # keep for Claude Code / VS Code
  http:
    enabled: true
    listen: ":8080"
```

Both transports start simultaneously. The server is reachable at `http://<host>:8080/mcp`.

#### MCP protocol over HTTP

The HTTP transport uses JSON-RPC 2.0 over POST. Each workflow run performs three steps:

##### Step 1 — Initialize (creates a session)

```json
POST http://server:8080/mcp
Content-Type: application/json

{
  "jsonrpc": "2.0", "id": 1, "method": "initialize",
  "params": {
    "protocolVersion": "2025-03-26",
    "capabilities": {},
    "clientInfo": { "name": "n8n", "version": "1.0" }
  }
}
```

The response contains the header `Mcp-Session-Id: <uuid>` — save this for subsequent requests.

##### Step 2 — Initialized notification

```json
POST http://server:8080/mcp
Mcp-Session-Id: <uuid>

{"jsonrpc": "2.0", "method": "notifications/initialized"}
```

##### Step 3 — Call a tool

```json
POST http://server:8080/mcp
Mcp-Session-Id: <uuid>

{
  "jsonrpc": "2.0", "id": 2, "method": "tools/call",
  "params": {
    "name": "zosmf_list_jobs",
    "arguments": { "system": "MAINT", "owner": "*", "status": "ACTIVE" }
  }
}
```

#### n8n workflow structure

```
[Trigger]
  → [HTTP POST /mcp  initialize]
  → [Set: sessionId from response header]
  → [HTTP POST /mcp  notifications/initialized]
  → [HTTP POST /mcp  tools/call]
  → [Process result ...]
  → [HTTP DELETE /mcp]   ← optional, cleans up session
```

#### Notes

- No TLS is built into the HTTP transport — use nginx or Traefik as a reverse proxy for production.
- The n8n host must be able to reach the MCP server on the configured port.
- Credentials (z/OSMF passwords) stay on the MCP server — n8n never has direct mainframe access.

---

## 5. Tools Overview

The server provides 32 tools across 8 categories:

### Jobs (9 tools)
| Tool                      | Description                                  |
|---------------------------|----------------------------------------------|
| `zosmf_list_jobs`         | List jobs with optional filters              |
| `zosmf_get_job`           | Get details of a specific job                |
| `zosmf_get_job_output`    | List spool files (DD names) of a job         |
| `zosmf_get_spool_content` | Read the content of a spool file             |
| `zosmf_submit_job`        | Submit a job from inline JCL or a dataset    |
| `zosmf_cancel_job`        | Cancel a running job                         |
| `zosmf_purge_job`         | Delete job output from the spool             |
| `zosmf_hold_job`          | Place a job on hold                          |
| `zosmf_release_job`       | Release a held job                           |

### SMP/E (1 tool)
| Tool                   | Description                                      |
|------------------------|--------------------------------------------------|
| `zosmf_smpe_query_csi` | Query the SMP/E CSI (SYSMODs, DDDEFs, products) |

### Datasets (8 tools)

| Tool                         | Description                                              |
|------------------------------|----------------------------------------------------------|
| `zosmf_list_datasets`        | Search datasets by pattern                               |
| `zosmf_list_members`         | List members of a PDS or PDSE                            |
| `zosmf_read_dataset`         | Read a sequential dataset or PDS member                  |
| `zosmf_write_dataset`        | Write to a sequential dataset or PDS member              |
| `zosmf_create_dataset`       | Allocate a new dataset                                   |
| `zosmf_delete_dataset`       | Delete a dataset or PDS member                           |
| `zosmf_acquire_dataset_enq`  | Acquire an exclusive or shared ENQ lock on a dataset or member |
| `zosmf_release_dataset_enq`  | Release a previously acquired ENQ lock                   |

### Filesystems (5 tools)
| Tool                       | Description                              |
|----------------------------|------------------------------------------|
| `zosmf_list_filesystems`   | List mounted filesystems                 |
| `zosmf_mount_filesystem`   | Mount a filesystem                       |
| `zosmf_unmount_filesystem` | Unmount a filesystem                     |
| `zosmf_create_zfs`         | Create a new zFS dataset                 |
| `zosmf_delete_zfs`         | Delete a zFS dataset                     |

### Operator Console (2 tools)
| Tool                     | Description                              |
|--------------------------|------------------------------------------|
| `zosmf_issue_command`    | Issue an operator command                |
| `zosmf_get_hardcopy_log` | Read OPERLOG or SYSLOG                   |

### USS Files (5 tools)
| Tool                     | Description                              |
|--------------------------|------------------------------------------|
| `zosmf_list_uss_files`   | List a USS directory                     |
| `zosmf_read_uss_file`    | Read a USS file                          |
| `zosmf_write_uss_file`   | Write to a USS file                      |
| `zosmf_create_uss_file`  | Create a USS file or directory           |
| `zosmf_delete_uss_file`  | Delete a USS file or directory           |

### TSO (1 tool)
| Tool                  | Description                                      |
|-----------------------|--------------------------------------------------|
| `zosmf_tso_command`   | Execute a TSO/E command and return the output    |

### Parmlib (1 tool)

| Tool                     | Description                                    |
|--------------------------|------------------------------------------------|
| `zosmf_parmlib_validate` | Validate z/OS parmlib member syntax via z/OSMF |

---

## 6. Tool Reference and Example Prompts

### Jobs

**Show all active jobs**
```
Show me all currently active jobs on SYSA
```

**Jobs for a specific owner**
```
Which jobs are currently running under user IBMUSER on SYSA?
```

**All OUTPUT jobs as a table**
```
List all jobs in OUTPUT status on SYSA and show the result as a table
```

**Read job output**
```
Show me the content of the JESMSGLG spool file for job MYJOB JOB12345 on SYSA
```

**Submit a job**
```
Submit the following job on SYSA:
//TESTJOB JOB (ACCT),'Test',CLASS=A,MSGCLASS=X
//STEP1   EXEC PGM=IEFBR14
```

**Submit a job from a dataset**
```
Submit the job from dataset USER.JCL(MYJOB) on SYSA
```

**Cancel a job**
```
Cancel job LONGJOB JOB00042 on SYSA
```

**Delete job output**
```
Delete the spool output of job TESTJOB JOB12345 on SYSA
```

---

### SMP/E

**Check whether a SYSMOD is installed**
```
Is SYSMOD UA12345 installed on SYSA in target zone TGTZONE?
```

**Show all SYSMODs for a FMID**
```
Show me all installed SYSMODs for FMID HBB77E0 on SYSA
```

**Look up a DDDEF dataset name**
```
What is the dataset name of DDDEF entry SYSLIB in the target zone on SYSA?
```

**Show all target zones**
```
Show me all configured target zones in the CSI on SYSA
```

**Query product information**
```
Show me all installed products on SYSA
```

**Search for error HOLDDATAs**
```
Are there any open HOLDDATAs of type ERROR on SYSA in the target zone?
```

---

### Datasets

**Search datasets by pattern**
```
Find all datasets starting with SYS1.PARMLIB on SYSA
```

**List PDS members**
```
List all members of SYS1.PARMLIB on SYSA
```

**Read dataset content**
```
Show me the content of SYS1.PARMLIB(IEASYS00) on SYSA
```

**Write a PDS member**
```
Write the following content to USER.JCL(NEWJOB) on SYSA: [JCL...]
```

**Allocate a new dataset**
```
Allocate a new PDS dataset USER.TEST.PDS on SYSA with
DSORG=PO, RECFM=FB, LRECL=80, 5 tracks primary, 1 track secondary, 10 directory blocks
```

**Delete a dataset**
```
Delete dataset USER.TEMP.DATA on SYSA
```

---

### Filesystems

**Show all mounted filesystems**
```
Show me all filesystems mounted on SYSA
```

**Filter filesystems by mount path**
```
Which filesystems are mounted under /usr on SYSA?
```

**Mount a filesystem**
```
Mount zFS dataset USER.MY.ZFS on SYSA at path /u/myuser/work in read-write mode
```

**Create a new zFS**
```
Create a new zFS dataset USER.NEW.ZFS on SYSA with 50 primary cylinders
```

---

### Operator Console

**Query system time**
```
What time is it on SYSA right now?
```
*(issues operator command `D T`)*

**Show all active address spaces**
```
Show me all active address spaces on SYSA
```
*(issues `D A,L`)*

**Check CPU utilization**
```
Show me the current CPU utilization on SYSA
```
*(issues `D M=CPU`)*

**Start a started task**
```
Start the started task MYAPP on SYSA
```
*(issues `S MYAPP`)*

**Retrieve the last 30 minutes of OPERLOG**
```
Show me the operator log messages from the last 30 minutes on SYSA
```

**Search SYSLOG for errors**
```
Are there any error messages in the SYSLOG on SYSA from the last 10 minutes?
```

---

### USS Files

**List a directory**
```
List the contents of directory /u/ibmuser on SYSA
```

**Read a file**
```
Show me the content of /u/ibmuser/.profile on SYSA
```

**Write a file**
```
Write the following content to /u/ibmuser/test.sh on SYSA: [script...]
```

**Create a new file**
```
Create the file /u/ibmuser/config.txt on SYSA
```

**Create a new directory**
```
Create directory /u/ibmuser/projects on SYSA with permissions rwxr-xr-x
```

**Delete a file**
```
Delete the file /u/ibmuser/old.log on SYSA
```

---

### TSO

**Query the dataset catalog**
```
Run the TSO command LISTCAT LVL(USER) on SYSA
```

**Show dataset details**
```
Use TSO on SYSA to show details for dataset USER.JCL
```
*(issues `LISTDS USER.JCL`)*

**Check RACF permissions**
```
Use TSO on SYSA to check RACF permissions for dataset USER.SECURE.DATA
```
*(issues `LISTDSD DA(USER.SECURE.DATA) ALL`)*

---

### Parmlib

#### Validate all active BPXPRM members

```
Validate the syntax of all active BPXPRM parmlib members on SYSA
```

#### Validate a specific member

```
Validate the syntax of SYS1.PARMLIB(IEASYS00) on SYSA
```

#### Validate all parmlib types via LOADxx

```
Validate all parmlib types deeply using LOADxx member LOAD00 from SYS1.PARMLIB on SYSA
```

#### Validate inline content

```
Validate the following BPXPRM parmlib content on SYSA: [parmlib text...]
```

---

## 7. Building Automations

The zosmf-mcp server can be used with Claude Code to build complex automations.

### Simple Automation: Job Monitor

Prompt:
```
Monitor all active jobs on SYSA every 5 minutes and notify me
when a job completes with a return code greater than 0.
Create a shell script monitor.sh that performs this check.
```

### Complex Automation: Patch Verification

Prompt:
```
Check whether the following SYSMODs are installed on SYSA: UA12345, UA12346, UA12347.
Create a table showing the status (installed / not installed),
installation date, and the corresponding FMID for each.
```

### Deployment Pipeline

Prompt:
```
Perform the following steps on SYSA:
1. Read the JCL template from SYS1.PROCLIB(DEPLOY)
2. Replace &VERSION with "V2R1" and &DATE with today's date
3. Submit the modified job
4. Wait for completion and show me the return code
5. If RC > 0, show me the JESMSGLG spool file
```

### System Health Check

Prompt:
```
Run a full health check on SYSA:
1. System time and date
2. All active address spaces
3. Mounted filesystems
4. OPERLOG from the last 5 minutes — check for error messages
Summarize the results as a report.
```

### Multiple Systems

When multiple systems are configured, they can be compared in a single prompt:

```
Compare the installed version of SYSMOD HBB77E0 on SYSA and SYSB.
Are both systems at the same maintenance level?
```

### Claude Code Hooks

With Claude Code hooks, actions can be triggered automatically. Example `.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "zosmf_submit_job",
        "hooks": [
          {
            "type": "command",
            "command": "echo 'Job submitted at $(date)' >> job.log"
          }
        ]
      }
    ]
  }
}
```

### Scheduled Tasks with Claude Code

Use the `/schedule` command in Claude Code to set up recurring tasks:

```
/schedule daily at 08:00 AM:
Check on SYSA whether all critical started tasks (TCPIP, VTAM, JES2) are active.
If any are missing, generate an alert.
```

---

## Additional Notes

- **TLS**: For production systems, set `skip_tls_verify: false` and configure CA certificates correctly.
- **Permissions**: The configured z/OSMF user requires appropriate SAF/RACF permissions for the z/OSMF REST APIs being used.
- **TSO**: `tso_account` and `tso_proc` must be set in the configuration to use `zosmf_tso_command`. The logon procedure must be valid on the target system (e.g. `IZUFPROC`).
- **SMP/E**: If `csi_dataset` and `zone` are not specified explicitly, the default values from the system configuration are used.
- **Passwords**: Never hardcode passwords in the configuration file. Always use environment variables (`${VAR}`).
