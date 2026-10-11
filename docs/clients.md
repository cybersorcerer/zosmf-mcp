# Registering the server with an MCP client

One section per client. Each needs the path to the binary and to the configuration file. Passwords are not part of any client configuration: the server reads them from the `.env` next to its configuration file, which `zosmf-mcp init env` writes.

[← README](../README.md)


zosmf-mcp supports three transports that can run simultaneously:

- **stdio** — the client starts the server process and communicates via stdin/stdout. Used by Claude Desktop, Claude Code, VS Code, etc.
- **HTTP** — the server listens on a TCP port and accepts JSON-RPC 2.0 requests. Used by n8n HTTP Request nodes, curl, and custom clients.
- **Streamable HTTP** — MCP 2025-03-26 compliant transport with SSE support. Used by the n8n MCP Client Tool Node (v1.88+).

All transports are controlled via the config file. The `--transport` flag can override this (`stdio`, `http`, `streamable-http`, or `auto` which reads the config — default is `auto`).

For the stdio clients the configuration is the same everywhere:

- **Command**: path to the `zosmf-mcp` binary
- **Arguments**: `--config` and the full path of the configuration file — `~/.config/smpe_ls/.smpe-zosmf.yaml` unless you chose another
- **Environment variables**: none needed. The server reads the passwords from the `.env` next to the configuration file; a variable a client does pass wins over the file.


## Claude Desktop (macOS/Windows)

Claude Desktop supports MCP servers via the `claude_desktop_config.json` file.

**Location:**

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "zosmf-mcp": {
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "/Users/<user>/.config/smpe_ls/.smpe-zosmf.yaml"]
    }
  }
}
```

> **Tip:** On macOS, specify the full path to the binary since Claude Desktop does not perform shell PATH expansion. Use `which zosmf-mcp` to find the path.

Restart Claude Desktop after saving. The tools will appear in the chat as available functions (hammer icon).


## Claude Code CLI

Register the server for all your projects:

```bash
claude mcp add --scope user zosmf-mcp -- /Users/<user>/.local/bin/zosmf-mcp --config /Users/<user>/.config/smpe_ls/.smpe-zosmf.yaml
```

The scope decides where the server is available:

| `--scope` | Available in | Stored in |
| --- | --- | --- |
| `local` (the default) | the current project only | `~/.claude.json` |
| `project` | the current project, shared through version control | `.mcp.json` in the project root |
| `user` | all your projects | `~/.claude.json` |

`claude mcp remove zosmf-mcp --scope <scope>` removes it again.

**Check status:**
```bash
claude mcp list
# or in the interactive chat:
/mcp
```


## VS Code (Claude Code Extension)

MCP servers are registered in VS Code via an `mcp.json` file in the workspace directory.

**`.vscode/mcp.json`** (automatically detected by the extension):

```json
{
  "servers": {
    "zosmf-mcp": {
      "type": "stdio",
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "${workspaceFolder}/.smpe-zosmf.yaml"]
    }
  }
}
```

> `${workspaceFolder}` is resolved by VS Code to the current workspace directory. The server then reads the passwords from the `.env` next to that file.

**Check status:** Type `/mcp` in the chat window — all registered servers and their tools will be listed.


## JetBrains IDEs

JetBrains IDEs (IntelliJ, GoLand, etc.) support MCP via the **AI Assistant plugin**.

**Configuration:** `Settings → Tools → AI Assistant → Model Context Protocol (MCP)`

Add a new server entry:

| Field       | Value                                             |
|-------------|---------------------------------------------------|
| Name        | `zosmf-mcp`                                       |
| Command     | `/Users/<user>/.local/bin/zosmf-mcp`              |
| Arguments   | `--config /Users/<user>/.config/smpe_ls/.smpe-zosmf.yaml` |

Alternatively, as a JSON configuration in `~/.config/JetBrains/mcp.json` (since 2024.3):

```json
{
  "servers": {
    "zosmf-mcp": {
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "/Users/<user>/.config/smpe_ls/.smpe-zosmf.yaml"]
    }
  }
}
```


## Gemini CLI

Google Gemini CLI supports MCP servers via a `settings.json` configuration file.

**Location:** `~/.gemini/settings.json`

```json
{
  "mcpServers": {
    "zosmf-mcp": {
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "/Users/<user>/.config/smpe_ls/.smpe-zosmf.yaml"]
    }
  }
}
```

**Check status:** In the Gemini CLI session:
```
/mcp
```

All registered tools become available in the session — the model can use them as native capabilities.


## Continue.dev

[Continue](https://continue.dev) is an open-source AI coding extension for VS Code and JetBrains. MCP servers are configured in `~/.continue/config.json`:

```json
{
  "experimental": {
    "modelContextProtocolServers": [
      {
        "transport": {
          "type": "stdio",
          "command": "/Users/<user>/.local/bin/zosmf-mcp",
          "args": ["--config", "/Users/<user>/.config/smpe_ls/.smpe-zosmf.yaml"]
        }
      }
    ]
  }
}
```


## Cursor

[Cursor](https://cursor.sh) supports MCP servers since version 0.43 via `~/.cursor/mcp.json` (global) or `.cursor/mcp.json` in the project directory:

```json
{
  "mcpServers": {
    "zosmf-mcp": {
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "/Users/<user>/.config/smpe_ls/.smpe-zosmf.yaml"]
    }
  }
}
```

After saving, go to `Cursor Settings → MCP` — the server should appear with a green status indicator.


## Anthropic API (custom applications)

Using the Anthropic API with the **MCP Client SDK**, you can build custom applications that leverage Claude together with zosmf-mcp.

### Python — with `anthropic` SDK and `mcp` client

```bash
pip install anthropic mcp
```

```python
import asyncio
import anthropic
from mcp import StdioServerParameters
from mcp.client.stdio import stdio_client

SYSTEM = "SYSA"
CONFIG = "/Users/<user>/.config/smpe_ls/.smpe-zosmf.yaml"

async def main():
    server_params = StdioServerParameters(
        command="/Users/<user>/.local/bin/zosmf-mcp",
        args=["--config", CONFIG],
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

### TypeScript/Node.js

```bash
npm install @anthropic-ai/sdk @modelcontextprotocol/sdk
```

```typescript
import Anthropic from "@anthropic-ai/sdk";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";

const transport = new StdioClientTransport({
  command: "/Users/<user>/.local/bin/zosmf-mcp",
  args: ["--config", "/Users/<user>/.config/smpe_ls/.smpe-zosmf.yaml"],
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


## Direct invocation / custom clients

For custom applications or shell scripts, the server can be started directly and addressed via JSON-RPC 2.0 over stdio. A session has to start with `initialize` and the `notifications/initialized` notification — before that, the server answers every request with `server not initialized`.

**Example: list available tools**

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"shell","version":"1"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
  | zosmf-mcp --config ~/.config/smpe_ls/.smpe-zosmf.yaml --transport stdio
```

**Example: call a tool**

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"shell","version":"1"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"zosmf_issue_command","arguments":{"system":"SYSA","command":"D T"}}}' \
  | zosmf-mcp --config ~/.config/smpe_ls/.smpe-zosmf.yaml --transport stdio
```

Every request gets one JSON line in reply, the notification none.

MCP client SDKs are available for several languages:

- **Python**: `pip install mcp`
- **TypeScript/Node.js**: `npm install @modelcontextprotocol/sdk`
- **Go**: `github.com/mark3labs/mcp-go`


## n8n (HTTP transport)

[n8n](https://n8n.io) is an open-source workflow automation platform. It can call the zosmf-mcp server via the **HTTP transport** using standard HTTP Request nodes.

### Enable HTTP transport

In `.smpe-zosmf.yaml`:

```yaml
transport:
  stdio: true       # keep for Claude Code / VS Code
  http:
    enabled: true
    listen: ":8080"
```

Both transports start simultaneously. The server is reachable at `http://<host>:8080/mcp`.

An HTTP transport refuses to start without an API key. Generate one:

```bash
zosmf-mcp init env --generate-api-key
```

It writes `MCP_API_KEY` to the `.env` next to the configuration, which the server reads at startup. The command never prints the key — take it from that file for n8n. Every request has to send it as `Authorization: Bearer <key>`.

### MCP protocol over HTTP

The HTTP transport uses JSON-RPC 2.0 over POST. Each workflow run performs three steps:

#### Step 1 — Initialize (creates a session)

```json
POST http://server:8080/mcp
Content-Type: application/json
Authorization: Bearer <key>

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

#### Step 2 — Initialized notification

```json
POST http://server:8080/mcp
Authorization: Bearer <key>
Mcp-Session-Id: <uuid>

{"jsonrpc": "2.0", "method": "notifications/initialized"}
```

#### Step 3 — Call a tool

```json
POST http://server:8080/mcp
Authorization: Bearer <key>
Mcp-Session-Id: <uuid>

{
  "jsonrpc": "2.0", "id": 2, "method": "tools/call",
  "params": {
    "name": "zosmf_list_jobs",
    "arguments": { "system": "MAINT", "owner": "*", "status": "ACTIVE" }
  }
}
```

### n8n workflow structure

```
[Trigger]
  → [HTTP POST /mcp  initialize]
  → [Set: sessionId from response header]
  → [HTTP POST /mcp  notifications/initialized]
  → [HTTP POST /mcp  tools/call]
  → [Process result ...]
  → [HTTP DELETE /mcp]   ← optional, cleans up session
```

### Notes

- No TLS is built into the HTTP transport — use nginx or Traefik as a reverse proxy for production.
- The n8n host must be able to reach the MCP server on the configured port.
- Credentials (z/OSMF passwords) stay on the MCP server — n8n never has direct mainframe access.


## n8n (MCP Client Node)

Since n8n **v1.88**, the native **MCP Client Tool Node** allows AI Agent workflows to use zosmf-mcp tools directly — without manually building HTTP Request nodes or managing session IDs.

### Enable Streamable HTTP transport

In `.smpe-zosmf.yaml`:

```yaml
transport:
  stdio: true                   # keep for Claude Code / VS Code
  streamable_http:
    enabled: true
    listen: ":8081"
```

Restart the server after changing the config. The MCP endpoint is then available at `http://<host>:8081/mcp`.

Like the HTTP transport, it needs an API key: run `zosmf-mcp init env --generate-api-key` once, as described [above](#enable-http-transport).

### n8n workflow setup

1. Add an **AI Agent** node to your workflow
2. Add a **MCP Client Tool** node and connect it to the Agent's tools input
3. Configure the MCP Client Tool node:
   - **Transport**: `Streamable HTTP` (n8n v1.104+) or `SSE` (older versions)
   - **URL**: `http://<host>:8081/mcp`
   - **Authentication**: `Bearer Auth`, with the key from `MCP_API_KEY` in the `.env`
4. The Agent can now use all zosmf-mcp tools natively

The MCP Client Node automatically handles session initialization, tool discovery, and tool calls. No manual JSON-RPC plumbing required.

### Requirements and Notes

- Use **Streamable HTTP** transport (n8n v1.104+) — it supersedes the older SSE transport.
- The standalone **MCP Client node** (n8n v1.122+) allows configuring the transport directly in node parameters.
- For production, use a reverse proxy (nginx, Traefik) for TLS termination in front of the Streamable HTTP port.


