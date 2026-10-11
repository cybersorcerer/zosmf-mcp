# Connecting an MCP client

[← README](../README.md)

There are two ways to connect:

- **The client starts the server** and talks to it over stdin and stdout. Desktop apps, CLIs and IDEs work this way.
- **The server runs on its own** and clients connect over HTTP — n8n, containers, or a server on another machine.

Contents:

- [Clients that start the server](#clients-that-start-the-server): [Claude Code](#claude-code), [Claude Desktop](#claude-desktop), [VS Code](#vs-code), [Cursor](#cursor), [Gemini CLI](#gemini-cli), [JetBrains IDEs](#jetbrains-ides), [Continue](#continue)
- [Clients that connect over HTTP](#clients-that-connect-over-http): [Claude Code](#claude-code-over-http), [n8n](#n8n-mcp-client-tool-node)
- [Building your own client](#building-your-own-client)

## Clients that start the server

Every such client needs the same two things — the program and its configuration file — and nothing else. Most of them read this format:

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

- **Use full paths.** Most clients do not search your shell's `PATH`. `which zosmf-mcp` prints the program's path; on Windows, `where zosmf-mcp`.
- **No passwords.** The server reads them from the `.env` next to the configuration file. A variable a client does pass wins over the file.

The sections below say where each client keeps this, and how to check that it connected.

### Claude Code

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

**Check:** `claude mcp list` shows `zosmf-mcp` as `✔ Connected`; in a session, `/mcp` lists it with its tools.

### Claude Desktop

In Claude Desktop, open **Settings → Developer → Edit Config**. That opens `claude_desktop_config.json`:

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

Add the `mcpServers` block shown [above](#clients-that-start-the-server), then quit Claude Desktop completely and start it again.

**Check:** the **Add files, connectors, and more** button at the bottom left of the chat input lists `zosmf-mcp` and its tools.

### VS Code

- **With the Claude Code extension:** the extension shares its MCP configuration with the Claude Code CLI. Register the server with `claude mcp add` as [above](#claude-code), for example in VS Code's integrated terminal.
- **With GitHub Copilot** (VS Code's own MCP support): put the `mcpServers` block shown [above](#clients-that-start-the-server) into `.mcp.json` at the root of your workspace, or into `~/.copilot/mcp-config.json` for all workspaces. A workspace `.mcp.json` is also what Claude Code reads for `--scope project`, so one file serves both.

**Check:** run **MCP: List Servers** from the Command Palette.

### Cursor

Put the `mcpServers` block shown [above](#clients-that-start-the-server) into `~/.cursor/mcp.json` for all projects, or `.cursor/mcp.json` for one. Cursor lists the server on its **Customize** page.

### Gemini CLI

Add the `mcpServers` block shown [above](#clients-that-start-the-server) to `~/.gemini/settings.json`.

**Check:** `/mcp` in a Gemini CLI session lists the server and its tools.

### JetBrains IDEs

The AI Assistant plugin connects to MCP servers. Open **Settings → Tools → AI Assistant → Model Context Protocol (MCP)**, select **Add**, and paste the `mcpServers` block shown [above](#clients-that-start-the-server) as JSON configuration.

### Continue

Create `.continue/mcpServers/zosmf-mcp.yaml` at the top of your workspace:

```yaml
name: zosmf-mcp
version: 0.0.1
schema: v1
mcpServers:
  - name: zosmf-mcp
    command: /Users/<user>/.local/bin/zosmf-mcp
    args:
      - --config
      - /Users/<user>/.config/smpe_ls/.smpe-zosmf.yaml
```

Continue also picks up the JSON block shown [above](#clients-that-start-the-server) as a file in the same directory. MCP tools are available in Continue's agent mode only.

## Clients that connect over HTTP

Here zosmf-mcp runs on its own — on this machine, another one, or [in a container](container-setup.md) — and clients reach it at `http://<host>:8081/mcp`. That needs the Streamable HTTP transport and an API key in the configuration; [Transports and API key](configuration.md#transports-and-api-key) shows both. In short:

```yaml
transport:
  streamable_http:
    enabled: true
    listen: ":8081"
```

```bash
zosmf-mcp init env --generate-api-key    # writes MCP_API_KEY to the .env next to the configuration
```

The command never prints the key — read it from that `.env` file. Every client sends it as `Authorization: Bearer <key>`.

The HTTP transports carry no TLS: anyone who can read the traffic sees the key and every z/OS command. Keep the port on a trusted network, or put a TLS-terminating reverse proxy in front.

### Claude Code over HTTP

```bash
claude mcp add --transport http --scope user zosmf-mcp http://<host>:8081/mcp \
  --header "Authorization: Bearer <key>"
```

### n8n: MCP Client Tool node

1. Add an **AI Agent** node to your workflow.
2. Add an **MCP Client Tool** node and connect it to the agent's tools input.
3. Configure the MCP Client Tool node:
   - **Transport**: `Streamable HTTP` (n8n v1.104+) or `SSE` (older versions)
   - **URL**: `http://<host>:8081/mcp`
   - **Authentication**: `Bearer Auth`, with the key from `MCP_API_KEY`
4. The agent can now use all zosmf-mcp tools.

The node handles session setup, tool discovery and tool calls itself.

### n8n: HTTP Request nodes

Without the MCP Client Tool node, a workflow can speak the protocol with plain **HTTP Request** nodes. This uses the older `http` transport — `transport.http.enabled: true`, default port `8080` — with the same API key. Each run takes three requests:

**1 — Initialize** (creates a session):

```json
POST http://<host>:8080/mcp
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

The response carries the header `Mcp-Session-Id: <uuid>` — keep it for the next requests.

**2 — Initialized notification:**

```json
POST http://<host>:8080/mcp
Authorization: Bearer <key>
Mcp-Session-Id: <uuid>

{"jsonrpc": "2.0", "method": "notifications/initialized"}
```

**3 — Call a tool:**

```json
POST http://<host>:8080/mcp
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

A workflow therefore looks like this:

```
[Trigger]
  → [HTTP POST /mcp  initialize]
  → [Set: sessionId from response header]
  → [HTTP POST /mcp  notifications/initialized]
  → [HTTP POST /mcp  tools/call]
  → [Process result ...]
  → [HTTP DELETE /mcp]   ← optional, ends the session
```

The z/OSMF passwords stay on the zosmf-mcp host; n8n never reaches the mainframe directly.

## Building your own client

### Over stdio, from a shell

A session has to start with `initialize` and the `notifications/initialized` notification — before that, the server answers every request with `server not initialized`.

**List the tools:**

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"shell","version":"1"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
  | zosmf-mcp --config ~/.config/smpe_ls/.smpe-zosmf.yaml --transport stdio
```

**Call a tool:**

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"shell","version":"1"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"zosmf_issue_command","arguments":{"system":"SYSA","command":"D T"}}}' \
  | zosmf-mcp --config ~/.config/smpe_ls/.smpe-zosmf.yaml --transport stdio
```

Every request gets one JSON line in reply, the notification none.

### With the Anthropic API

Both examples start zosmf-mcp over stdio, hand its tools to Claude, and run the tool calls Claude asks for until it has an answer.

**Python**

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
                    model="claude-opus-5-5",
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

**TypeScript**

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
    model: "claude-opus-5-5",
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

### MCP SDKs

- **Python**: `pip install mcp`
- **TypeScript**: `npm install @modelcontextprotocol/sdk`
- **Go**: `github.com/modelcontextprotocol/go-sdk`
