# zosmf-mcp — User Guide

Ein [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) Server für z/OS, der Claude und anderen KI-Assistenten direkten Zugriff auf z/OS-Systeme über die z/OSMF REST API ermöglicht.

---

## Inhaltsverzeichnis

1. [Voraussetzungen](#1-voraussetzungen)
2. [Installation](#2-installation)
3. [Konfiguration](#3-konfiguration)
4. [MCP Server registrieren](#4-mcp-server-registrieren)
   - [Claude Desktop](#41-claude-desktop-macoswindows)
   - [Claude Code CLI](#42-claude-code-cli)
   - [VS Code (Claude Code Extension)](#43-vs-code-claude-code-extension)
   - [JetBrains IDEs](#44-jetbrains-ides)
   - [Gemini CLI](#45-gemini-cli)
   - [Continue.dev](#46-continuedev)
   - [Cursor](#47-cursor)
   - [Anthropic API (eigene Applikationen)](#48-anthropic-api-eigene-applikationen)
   - [Direkter Aufruf / eigene Clients](#49-direkter-aufruf--eigene-clients)
   - [n8n (HTTP-Transport)](#410-n8n-http-transport)
   - [n8n (MCP Client Node)](#411-n8n-mcp-client-node)
5. [Tools Übersicht](#5-tools-übersicht)
6. [Tool-Referenz und Beispiel-Prompts](#6-tool-referenz-und-beispiel-prompts)
7. [Automatisierungen erstellen](#7-automatisierungen-erstellen)

---

## 1. Voraussetzungen

- **Go 1.22+** — zum Selbst-Kompilieren aus dem Quellcode
- **z/OSMF** — aktiviert auf dem Zielsystem (z/OS V2.3+)
- **Netzwerkzugang** — zum z/OSMF-Port (Standard: 443)
- **VS Code** mit **Claude Code Extension** — für die MCP-Integration

---

## 2. Installation

### Aus dem Quellcode bauen

```bash
git clone https://gitlab.com/ronaldfunk/zosmf-mcp.git
cd zosmf-mcp
make install
```

Der Binary wird nach `~/.local/bin/zosmf-mcp` installiert. Sicherstellen, dass dieses Verzeichnis im `PATH` enthalten ist:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

### Verfügbare Make-Targets

| Target      | Beschreibung                            |
| ----------- | --------------------------------------- |
| `build`     | Binary bauen                            |
| `install`   | Binary nach `~/.local/bin` installieren |
| `test`      | Tests mit Race Detector ausführen       |
| `vet`       | `go vet` ausführen                      |
| `fmt`       | Quellcode formatieren                   |
| `lint`      | `golangci-lint` ausführen               |
| `clean`     | Binary und Caches löschen               |
| `tidy`      | `go mod tidy` ausführen                 |
---

## 3. Konfiguration

Die Konfiguration erfolgt über eine YAML-Datei. Standardmäßig sucht der Server nach `.zosmf-mcp-config.yaml` im Arbeitsverzeichnis.

### Beispielkonfiguration

```yaml
systems:
  - name: "SYSA"
    host: "<HOSTNAME>:<PORT>"
    auth:
      type: "basic"
      username: "IBMUSER"
      password: "${ZOSMF_MAINT_PASSWORD}"  # Aus Umgebungsvariable
    skip_tls_verify: true                  # Für Produktion: false setzen und ca_file konfigurieren
    # ca_file: "/path/to/ca.pem"           # Custom CA für selbstsignierte Zertifikate
    csi:                                   # Standard-CSI-Datasets für SMP/E
      - "SMPE.GLOBAL.CSI"
    zones:                                 # Standard SMP/E Zonen
      - "MVST100"
      - "MVSD100"
    tso_account:  "ACCT#"                 # TSO Account-Nummer
    tso_proc:     "IZUFPROC"              # TSO Logon-Prozedur
    tso_charset:  "697"                   # TSO Character Set (Standard: 697)
    tso_codepage: "1047"                  # TSO Code Page (Standard: 1047)

  # Zweites System mit Zertifikats-Authentifizierung
  # - name: "PROD"
  #   host: "https://prod.example.com:443"
  #   auth:
  #     type: "certificate"
  #     cert_file: "/path/to/client.pem"
  #     key_file:  "/path/to/client.key"
  #   ca_file: "/path/to/ca.pem"

# SSH-Systeme für ZOAU-Tools (optional — weglassen wenn ZOAU nicht benötigt wird)
# ssh_systems:
#   - name: "MVSA"
#     host: "mvsa.example.com"
#     port: 22                    # optional, Standard: 22
#     username: "ibmuser"
#     key_file: "~/.ssh/id_rsa"  # optional — weglassen für SSH-Standard-Key

transport:
  stdio: true                   # Für MCP-Integration (VS Code, Claude Code)
  http:
    enabled: false              # Optional: HTTP-Transport für n8n HTTP Request Nodes, curl etc.
    listen: ":8080"
  streamable_http:
    enabled: false              # Optional: Streamable HTTP für n8n MCP Client Tool Node (ab v1.88)
    listen: ":8081"
```

### Konfigurationsoptionen

| Feld               | Pflicht   | Beschreibung                                                     |
| ------------------ | --------- | ---------------------------------------------------------------- |
| `name`             | Ja        | Systemname (wird in Prompts verwendet, z.B. "MAINT")             |
| `host`             | Ja        | z/OSMF Base URL inkl. Port                                       |
| `auth.type`        | Ja        | `basic` oder `certificate`                                       |
| `auth.username`    | Ja*       | TSO-Benutzerkennung (*bei `basic`)                               |
| `auth.password`    | Ja*       | Passwort, kann als `${ENV_VAR}` referenziert werden              |
| `ca_file`          | Nein      | PEM-Datei mit der CA für selbstsignierte z/OSMF-Zertifikate      |
| `skip_tls_verify`  | Nein      | TLS-Validierung überspringen (nicht für Produktion empfohlen)    |
| `csi`              | Nein      | Standard-CSI-Datasets für SMP/E-Abfragen                         |
| `zones`            | Nein      | Standard SMP/E Zonen (Fallback wenn keine Zone angegeben)        |
| `tso_account`      | Nein      | TSO Account-Nummer (für `zosmf_tso_command`)                     |
| `tso_proc`         | Nein      | TSO Logon-Prozedur (für `zosmf_tso_command`)                     |
| `tso_charset`      | Nein      | TSO Character Set, Standard: `697`                               |
| `tso_codepage`     | Nein      | TSO Code Page, Standard: `1047`                                  |
### Passwort sicher übergeben

Passwörter sollten nie in Klartext in der Konfigurationsdatei stehen. Umgebungsvariablen verwenden:

```bash
# .env-Datei (nicht in Git committen!)
export ZOSMF_MAINT_PASSWORD="mein_passwort"

# Vor dem Start laden:
source .env
```

In der Config dann:
```yaml
password: "${ZOSMF_MAINT_PASSWORD}"
```

Falls `auth.password` nicht gesetzt ist und kein `${ENV_VAR}`-Platzhalter vorhanden ist, greift der Server auf die Umgebungsvariable `ZOSMF_PASSWORD` als globalen Fallback zurück.

### SSH-Systeme (für ZOAU-Tools)

ZOAU-Tools benötigen SSH-Zugriff auf einzelne LPARs. SSH-Systeme werden unabhängig von z/OSMF-Systemen konfiguriert — ein z/OSMF-Endpoint kann den gesamten Sysplex sehen, SSH benötigt aber einen separaten Eintrag pro LPAR.

```yaml
ssh_systems:
  - name: "MVSA"
    host: "mvsa.example.com"
    port: 22                    # optional, Standard: 22
    username: "ibmuser"
    key_file: "~/.ssh/id_rsa"  # optional — weglassen für SSH-Standard-Key (~/.ssh)
  - name: "MVSB"
    host: "mvsb.example.com"
    username: "ibmuser"
```

ZOAU-Tools werden nur registriert wenn mindestens ein `ssh_systems`-Eintrag vorhanden ist. Passwort-basiertes SSH wird nicht unterstützt — nur SSH-Key-Authentifizierung.

#### SSH-Systemkonfiguration

| Feld         | Pflicht   | Standard   | Beschreibung                                                |
| ------------ | --------- | ---------- | ----------------------------------------------------------- |
| `name`       | Ja        | —          | LPAR-Name — wird als `system` in Tool-Aufrufen verwendet    |
| `host`       | Ja        | —          | SSH-Hostname oder IP-Adresse                                |
| `port`       | Nein      | 22         | SSH-Port                                                    |
| `username`   | Ja        | —          | USS-Benutzername für SSH-Login                              |
| `key_file`   | Nein      | —          | Pfad zum SSH-Private-Key; weglassen für SSH-Standard-Key    |
---

## 4. MCP Server registrieren

zosmf-mcp unterstützt drei Transporte, die gleichzeitig laufen können:

- **stdio** — der Client startet den Server-Prozess selbst und kommuniziert über stdin/stdout. Verwendet von Claude Desktop, Claude Code, VS Code etc.
- **HTTP** — der Server lauscht auf einem TCP-Port und nimmt JSON-RPC 2.0 Requests entgegen. Verwendet von n8n HTTP Request Nodes, curl und eigenen Clients.
- **Streamable HTTP** — MCP 2025-03-26 konformer Transport mit SSE-Unterstützung. Verwendet von der n8n MCP Client Tool Node (ab v1.88).

Alle Transporte werden über die Config-Datei gesteuert. Der `--transport`-Flag kann das überschreiben (`stdio`, `http`, `streamable-http` oder `auto` liest die Config — Standard ist `auto`).

Für die stdio-Clients gilt überall dasselbe Prinzip:

- **Befehl**: Pfad zum `zosmf-mcp`-Binary
- **Argumente**: `--config /pfad/zur/.zosmf-mcp-config.yaml`
- **Umgebungsvariablen**: Passwörter als `ENV`-Variablen übergeben

---

### 4.1 Claude Desktop (macOS/Windows)

Claude Desktop unterstützt MCP-Server über die Datei `claude_desktop_config.json`.

**Speicherort:**

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "zosmf-mcp": {
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "/Users/<user>/.zosmf-mcp-config.yaml"],
      "env": {
        "ZOSMF_SYSA_PASSWORD": "geheim"
      }
    }
  }
}
```

> **Tipp:** Auf macOS den vollen Pfad zum Binary angeben, da Claude Desktop keine Shell-PATH-Expansion durchführt. Mit `which zosmf-mcp` den Pfad ermitteln.

Nach dem Speichern Claude Desktop neu starten. Die Tools erscheinen dann im Chat als verfügbare Funktionen (Hammer-Symbol).

---

### 4.2 Claude Code CLI

Claude Code (das CLI-Tool `claude`) liest MCP-Server-Konfigurationen aus `~/.claude/mcp.json` (global) oder `.claude/mcp.json` im Projektverzeichnis (lokal).

**Global registrieren** (gilt für alle Projekte):

```bash
claude mcp add zosmf-mcp \
  /Users/<user>/.local/bin/zosmf-mcp \
  -- --config /Users/<user>/.zosmf-mcp-config.yaml
```

Oder manuell in `~/.claude/mcp.json`:

```json
{
  "mcpServers": {
    "zosmf-mcp": {
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "/Users/<user>/.zosmf-mcp-config.yaml"],
      "env": {
        "ZOSMF_SYSA_PASSWORD": "geheim"
      }
    }
  }
}
```

**Lokal für ein Projekt** (`.claude/mcp.json` im Repo):

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

**Status prüfen:**
```bash
claude mcp list
# oder im interaktiven Chat:
/mcp
```

---

### 4.3 VS Code (Claude Code Extension)

MCP-Server werden in VS Code über eine `mcp.json` im Workspace-Verzeichnis registriert.

**`.vscode/mcp.json`** (wird automatisch von der Extension erkannt):

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

> `${workspaceFolder}` wird von VS Code auf das aktuelle Workspace-Verzeichnis aufgelöst. `${env:VAR}` liest Umgebungsvariablen aus dem Terminal, in dem VS Code gestartet wurde.

**Status prüfen:** Im Chat-Fenster `/mcp` eingeben — alle registrierten Server und deren Tools werden aufgelistet.

---

### 4.4 JetBrains IDEs

JetBrains IDEs (IntelliJ, GoLand, etc.) unterstützen MCP über das **AI Assistant Plugin**.

**Konfiguration:** `Settings → Tools → AI Assistant → Model Context Protocol (MCP)`

Dort einen neuen Server hinzufügen:

| Feld          | Wert                                                |
| ------------- | --------------------------------------------------- |
| Name          | `zosmf-mcp`                                         |
| Command       | `/Users/<user>/.local/bin/zosmf-mcp`                |
| Arguments     | `--config /Users/<user>/.zosmf-mcp-config.yaml`     |
| Environment   | `ZOSMF_SYSA_PASSWORD=geheim`                        |
Alternativ als JSON-Konfiguration unter `~/.config/JetBrains/mcp.json` (ab 2024.3):

```json
{
  "servers": {
    "zosmf-mcp": {
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "/Users/<user>/.zosmf-mcp-config.yaml"],
      "env": {
        "ZOSMF_SYSA_PASSWORD": "geheim"
      }
    }
  }
}
```

---

### 4.5 Gemini CLI

Google Gemini CLI unterstützt MCP-Server über eine `settings.json`-Konfiguration.

**Speicherort:** `~/.gemini/settings.json`

```json
{
  "mcpServers": {
    "zosmf-mcp": {
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "/Users/<user>/.zosmf-mcp-config.yaml"],
      "env": {
        "ZOSMF_SYSA_PASSWORD": "geheim"
      }
    }
  }
}
```

**Status prüfen:** Im Gemini CLI:
```
/mcp
```

Alle registrierten Tools werden dann in der Session verfügbar — das Modell kann sie wie eigene Fähigkeiten einsetzen.

---

### 4.6 Continue.dev

[Continue](https://continue.dev) ist eine Open-Source KI-Coding-Erweiterung für VS Code und JetBrains. MCP-Server werden in `~/.continue/config.json` konfiguriert:

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
            "ZOSMF_SYSA_PASSWORD": "geheim"
          }
        }
      }
    ]
  }
}
```

---

### 4.7 Cursor

[Cursor](https://cursor.sh) unterstützt MCP-Server ab Version 0.43 über `~/.cursor/mcp.json` (global) oder `.cursor/mcp.json` im Projektverzeichnis:

```json
{
  "mcpServers": {
    "zosmf-mcp": {
      "command": "/Users/<user>/.local/bin/zosmf-mcp",
      "args": ["--config", "/Users/<user>/.zosmf-mcp-config.yaml"],
      "env": {
        "ZOSMF_SYSA_PASSWORD": "geheim"
      }
    }
  }
}
```

Nach dem Speichern in Cursor: `Cursor Settings → MCP` — der Server sollte dort mit grünem Status erscheinen.

---

### 4.8 Anthropic API (eigene Applikationen)

Mit der Anthropic API und dem `mcp-remote`-Adapter oder dem **MCP Client SDK** können eigene Applikationen gebaut werden, die Claude zusammen mit zosmf-mcp nutzen.

#### Python — mit `anthropic` SDK und `mcp` Client

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
        env={"ZOSMF_SYSA_PASSWORD": "geheim"},
    )

    async with stdio_client(server_params) as (read, write):
        from mcp.client.session import ClientSession
        async with ClientSession(read, write) as session:
            await session.initialize()

            # Verfügbare Tools als Anthropic-Tool-Definitionen laden
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
            messages = [{"role": "user", "content": f"Zeige mir die aktuelle Systemzeit auf {SYSTEM}"}]

            # Agentic Loop: Claude ruft Tools auf bis die Antwort vollständig ist
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

                # Tool-Aufrufe ausführen
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
  env: { ...process.env, ZOSMF_SYSA_PASSWORD: "geheim" },
});

const mcp = new Client({ name: "my-app", version: "1.0.0" }, { capabilities: {} });
await mcp.connect(transport);

// Tools laden
const { tools } = await mcp.listTools();
const anthropicTools = tools.map((t) => ({
  name: t.name,
  description: t.description ?? "",
  input_schema: t.inputSchema,
}));

const client = new Anthropic();
const messages: Anthropic.MessageParam[] = [
  { role: "user", content: "Zeige mir alle aktiven Jobs auf SYSA" },
];

// Agentic Loop
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

> Die Beispiele implementieren einen **Agentic Loop**: Claude entscheidet selbst welche Tools es aufruft, bis es eine vollständige Antwort formulieren kann.

---

### 4.9 Direkter Aufruf / eigene Clients

Für eigene Applikationen oder Shell-Skripte kann der Server direkt gestartet und per JSON-RPC 2.0 über stdio angesprochen werden:

```bash
# Server manuell starten (wartet auf JSON-RPC auf stdin)
ZOSMF_SYSA_PASSWORD=geheim zosmf-mcp --config .zosmf-mcp-config.yaml
```

**Beispiel: Tool-Liste abfragen**

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' \
  | ZOSMF_SYSA_PASSWORD=geheim zosmf-mcp --config .zosmf-mcp-config.yaml
```

**Beispiel: Tool aufrufen**

```bash
echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"zosmf_issue_command","arguments":{"system":"SYSA","command":"D T"}}}' \
  | ZOSMF_SYSA_PASSWORD=geheim zosmf-mcp --config .zosmf-mcp-config.yaml
```

Für die Integration in Python, Node.js oder andere Sprachen gibt es MCP-Client-SDKs:

- **Python**: `pip install mcp`
- **TypeScript/Node.js**: `npm install @modelcontextprotocol/sdk`
- **Go**: `github.com/mark3labs/mcp-go`

---

### 4.10 n8n (HTTP-Transport)

[n8n](https://n8n.io) ist eine Open-Source Workflow-Automatisierungsplattform. Sie kann den zosmf-mcp Server über den **HTTP-Transport** mit Standard HTTP Request Nodes aufrufen.

#### HTTP-Transport aktivieren

In `.zosmf-mcp-config.yaml`:

```yaml
transport:
  stdio: true       # für Claude Code / VS Code beibehalten
  http:
    enabled: true
    listen: ":8080"
```

Beide Transporte starten gleichzeitig. Der Server ist erreichbar unter `http://<host>:8080/mcp`.

#### MCP-Protokoll über HTTP

Der HTTP-Transport verwendet JSON-RPC 2.0 über POST. Jeder Workflow-Run führt drei Schritte durch:

##### Schritt 1 — Initialize (erstellt eine Session)

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

Die Response enthält den Header `Mcp-Session-Id: <uuid>` — diesen für alle folgenden Requests speichern.

##### Schritt 2 — Initialized Notification

```json
POST http://server:8080/mcp
Mcp-Session-Id: <uuid>

{"jsonrpc": "2.0", "method": "notifications/initialized"}
```

##### Schritt 3 — Tool aufrufen

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

#### n8n Workflow-Struktur

```
[Trigger]
  → [HTTP POST /mcp  initialize]
  → [Set: sessionId aus Response-Header]
  → [HTTP POST /mcp  notifications/initialized]
  → [HTTP POST /mcp  tools/call]
  → [Ergebnis verarbeiten ...]
  → [HTTP DELETE /mcp]   ← optional, räumt Session auf
```

#### Hinweise

- Kein TLS im HTTP-Transport eingebaut — für Produktion nginx oder Traefik als Reverse Proxy vorschalten.
- Der n8n-Host muss den MCP-Server auf dem konfigurierten Port erreichen können.
- Credentials (z/OSMF-Passwörter) bleiben auf dem MCP-Server — n8n hat keinen direkten Mainframe-Zugriff.

---

### 4.11 n8n (MCP Client Node)

Ab n8n **v1.88** ermöglicht die native **MCP Client Tool Node** es KI-Agent-Workflows, zosmf-mcp Tools direkt zu nutzen — ohne manuelle HTTP Request Nodes oder Session-ID-Verwaltung.

#### Streamable HTTP Transport aktivieren

In `.zosmf-mcp-config.yaml`:

```yaml
transport:
  stdio: true                   # für Claude Code / VS Code beibehalten
  streamable_http:
    enabled: true
    listen: ":8081"
```

Nach einer Konfigurationsänderung den Server neu starten. Der MCP-Endpoint ist dann erreichbar unter `http://<host>:8081/mcp`.

#### n8n Workflow einrichten

1. Einen **AI Agent** Node zum Workflow hinzufügen
2. Einen **MCP Client Tool** Node hinzufügen und mit dem Tools-Eingang des Agents verbinden
3. MCP Client Tool Node konfigurieren:
   - **Transport**: `Streamable HTTP` (n8n v1.104+) oder `SSE` (ältere Versionen)
   - **URL**: `http://<host>:8081/mcp`
4. Der Agent kann jetzt alle zosmf-mcp Tools nativ verwenden

Die MCP Client Node übernimmt automatisch Session-Initialisierung, Tool-Discovery und Tool-Aufrufe — kein manuelles JSON-RPC erforderlich.

#### Voraussetzungen und Hinweise

- **Streamable HTTP** Transport verwenden (n8n v1.104+) — löst den älteren SSE-Transport ab.
- Der eigenständige **MCP Client Node** (n8n v1.122+) erlaubt die Transport-Konfiguration direkt in den Node-Parametern.
- Für Produktion einen Reverse Proxy (nginx, Traefik) für TLS-Terminierung vor den Streamable HTTP Port schalten.

---

## 5. Tools Übersicht

Der Server stellt 33 Tools in 8 Kategorien bereit (plus 11 optionale ZOAU-Tools wenn `ssh_systems` konfiguriert ist):

### Jobs (9 Tools)
| Tool                      | Beschreibung                                |
| ------------------------- | ------------------------------------------- |
| `zosmf_list_jobs`         | Jobs auflisten (mit Filter)                 |
| `zosmf_get_job`           | Job-Details abrufen                         |
| `zosmf_get_job_output`    | Spool-Dateien eines Jobs auflisten          |
| `zosmf_get_spool_content` | Inhalt einer Spool-Datei lesen              |
| `zosmf_submit_job`        | Job aus JCL oder Dataset submitten          |
| `zosmf_cancel_job`        | Laufenden Job abbrechen                     |
| `zosmf_purge_job`         | Job-Output aus dem Spool löschen            |
| `zosmf_hold_job`          | Job in Hold setzen                          |
| `zosmf_release_job`       | Job aus Hold freigeben                      |
### SMP/E (1 Tool)
| Tool                      | Beschreibung                                |
| ------------------------- | ------------------------------------------- |
| `zosmf_smpe_query_csi`    | SMP/E CSI abfragen (SYSMODs, DDDEFs, ...)   |
### Datasets (8 Tools)
| Tool                         | Beschreibung                                                                               |
| ---------------------------- | ------------------------------------------------------------------------------------------ |
| `zosmf_list_datasets`        | Datasets nach Muster suchen; optional `volser` und `start` Parameter                      |
| `zosmf_list_members`         | Members eines PDS/PDSE auflisten                                                           |
| `zosmf_read_dataset`         | Dataset oder PDS-Member lesen                                                              |
| `zosmf_write_dataset`        | Dataset oder PDS-Member schreiben; optional `release_enq` zum ENQ-Freigeben nach dem Save |
| `zosmf_create_dataset`       | Neues Dataset anlegen                                                                      |
| `zosmf_delete_dataset`       | Dataset oder PDS-Member löschen                                                            |
| `zosmf_acquire_dataset_enq`  | Exklusiven oder geteilten ENQ-Lock auf Dataset oder Member setzen                         |
| `zosmf_release_dataset_enq`  | Zuvor gesetzten ENQ-Lock freigeben                                                         |
### Filesystems (5 Tools)
| Tool                        | Beschreibung                              |
| --------------------------- | ----------------------------------------- |
| `zosmf_list_filesystems`    | Gemountete Filesysteme auflisten          |
| `zosmf_mount_filesystem`    | Filesystem mounten                        |
| `zosmf_unmount_filesystem`  | Filesystem unmounten                      |
| `zosmf_create_zfs`          | Neues zFS Dataset anlegen                 |
| `zosmf_delete_zfs`          | zFS Dataset löschen                       |
### Operator Console (2 Tools)
| Tool                        | Beschreibung                              |
| --------------------------- | ----------------------------------------- |
| `zosmf_issue_command`       | Operator-Kommando ausführen               |
| `zosmf_get_hardcopy_log`    | OPERLOG oder SYSLOG lesen                 |
### USS Files (5 Tools)
| Tool                        | Beschreibung                              |
| --------------------------- | ----------------------------------------- |
| `zosmf_list_uss_files`      | USS-Verzeichnis auflisten                                                              |
| `zosmf_read_uss_file`       | USS-Datei lesen; optional `tag` zur Encoding-Auswahl (`t IBM-1047`, `b`)              |
| `zosmf_write_uss_file`      | USS-Datei schreiben; optional `tag` zur Encoding-Auswahl                              |
| `zosmf_create_uss_file`     | USS-Datei oder -Verzeichnis anlegen                                                    |
| `zosmf_delete_uss_file`     | USS-Datei oder -Verzeichnis löschen                                                    |
### TSO (1 Tool)

| Tool                    | Beschreibung                                      |
| ----------------------- | ------------------------------------------------- |
| `zosmf_tso_command`     | TSO/E-Kommando ausführen und Output zurückgeben   |
### Parmlib (1 Tool)

| Tool                       | Beschreibung                                          |
| -------------------------- | ----------------------------------------------------- |
| `zosmf_parmlib_validate`   | Syntax von z/OS Parmlib-Members validieren            |
### ZOAU (11 Tools) — erfordert `ssh_systems` Konfiguration

| Tool                        | Beschreibung                                                          |
| --------------------------- | --------------------------------------------------------------------- |
| `zosmf_zoau_pll`            | z/OS Linklist (LLA) Verkettung anzeigen                               |
| `zosmf_zoau_zinfo`          | z/OS Systeminformationen anzeigen                                     |
| `zosmf_zoau_llwhence`       | In welchem Linklist-Dataset ein Member enthalten ist                  |
| `zosmf_zoau_parmgrep`       | Parmlib-Members nach einem Muster durchsuchen                         |
| `zosmf_zoau_parmwhence`     | In welchem Parmlib-Dataset ein Member enthalten ist                   |
| `zosmf_zoau_pproc`          | z/OS Proclib Verkettung anzeigen                                      |
| `zosmf_zoau_procgrep`       | Proclib-Members nach einem Muster durchsuchen                         |
| `zosmf_zoau_procwhence`     | In welchem Proclib-Dataset ein Member enthalten ist                   |
| `zosmf_zoau_version`        | Installierte ZOAU-Version anzeigen                                    |
| `zosmf_zoau_vtocls`         | VTOC-Inhalt eines z/OS-Volumes auflisten                              |
| `zosmf_zoau_exec`           | USS-Befehl/Skript via SSH; stdout, stderr, Exit-Code als JSON         |
---

## 6. Tool-Referenz und Beispiel-Prompts

### Jobs

**Alle aktiven Jobs anzeigen**
```
Zeige mir alle derzeit aktiven Jobs auf SYSA
```

**Jobs eines bestimmten Besitzers**
```
Welche Jobs laufen gerade unter User IBMUSER auf SYSA?
```

**Alle Jobs im Output-Status als Tabelle**
```
Liste alle Jobs im Status OUTPUT auf SYSA und gib das Ergebnis als Tabelle aus
```

**Job-Output lesen**
```
Zeige mir den Inhalt der JESMSGLG-Spool-Datei von Job MYJOB JOB12345 auf SYSA
```

**Job submitten**
```
Submitte folgenden Job auf SYSA:
//TESTJOB JOB (ACCT),'Test',CLASS=A,MSGCLASS=X
//STEP1   EXEC PGM=IEFBR14
```

**Job aus Dataset submitten**
```
Submitte den Job aus Dataset USER.JCL(MYJOB) auf SYSA
```

**Job abbrechen**
```
Breche Job LONGJOB JOB00042 auf SYSA ab
```

**Job-Output löschen**
```
Lösche den Spool-Output von Job TESTJOB JOB12345 auf SYSA
```

---

### SMP/E

**SYSMOD prüfen ob installiert**
```
Ist der SYSMOD UA12345 auf SYSA in der Target-Zone TGTZONE installiert?
```

**Alle SYSMODs eines FMIDs anzeigen**
```
Zeige mir alle installierten SYSMODs des FMIDs HBB77E0 auf SYSA
```

**DDDEF Dataset-Namen herausfinden**
```
Was ist der Dataset-Name des DDDEF-Eintrags SYSLIB in der Target-Zone auf SYSA?
```

**Alle Target-Zonen anzeigen**
```
Zeige mir alle konfigurierten Target-Zonen im CSI auf SYSA
```

**Produkt-Informationen abfragen**
```
Zeige mir alle installierten Produkte (PRODUCT-Einträge) auf SYSA
```

**HOLDDATAs nach Fehler suchen**
```
Gibt es offene HOLDDATAs vom Typ ERROR auf SYSA in der Target-Zone?
```

---

### Datasets

**Datasets nach Muster suchen**
```
Suche alle Datasets die mit SYS1.PARMLIB beginnen auf SYSA
```

**Members eines PDS auflisten**
```
Liste alle Members von SYS1.PARMLIB auf SYSA
```

**Dataset-Inhalt lesen**
```
Zeige mir den Inhalt von SYS1.PARMLIB(IEASYS00) auf SYSA
```

**PDS-Member schreiben**
```
Schreibe folgenden Inhalt in USER.JCL(NEWJOB) auf SYSA: [JCL...]
```

**Neues Dataset anlegen**
```
Lege ein neues PDS Dataset USER.TEST.PDS auf SYSA an mit
DSORG=PO, RECFM=FB, LRECL=80, 5 Tracks primär, 1 Track sekundär, 10 Dirblöcke
```

**Dataset löschen**
```
Lösche das Dataset USER.TEMP.DATA auf SYSA
```

---

### Filesystems

**Alle gemounteten Filesysteme anzeigen**
```
Zeige mir alle auf SYSA gemounteten Filesysteme
```

**Filesysteme nach Pfad filtern**
```
Welche Filesysteme sind unter /usr auf SYSA gemountet?
```

**Filesystem mounten**
```
Mounte das zFS Dataset USER.MY.ZFS auf SYSA unter dem Pfad /u/myuser/work als rdwr
```

**Neues zFS anlegen**
```
Lege ein neues zFS Dataset USER.NEW.ZFS auf SYSA an mit 50 Primär-Zylindern
```

---

### Operator Console

**Systemzeit abfragen**
```
Wie spät ist es gerade auf SYSA?
```
*(entspricht Kommando `D T`)*

**Alle aktiven Adressen-Spaces anzeigen**
```
Zeige mir alle aktiven Address Spaces auf SYSA
```
*(entspricht `D A,L`)*

**CPU-Auslastung abfragen**
```
Zeige mir die aktuelle CPU-Auslastung auf SYSA
```
*(entspricht `D M=CPU`)*

**Started Task starten**
```
Starte den Started Task MYAPP auf SYSA
```
*(entspricht `S MYAPP`)*

**OPERLOG der letzten 30 Minuten**
```
Zeige mir die Operator-Log-Meldungen der letzten 30 Minuten von SYSA
```

**SYSLOG nach Fehlern durchsuchen**
```
Gibt es in den letzten 10 Minuten Fehlermeldungen im SYSLOG auf SYSA?
```

---

### USS Files

**Verzeichnis auflisten**
```
Liste den Inhalt des Verzeichnisses /u/ibmuser auf SYSA
```

**Datei lesen**
```
Zeige mir den Inhalt von /u/ibmuser/.profile auf SYSA
```

**Datei schreiben**
```
Schreibe folgenden Inhalt in /u/ibmuser/test.sh auf SYSA: [Script...]
```

**Neue Datei anlegen**
```
Erstelle die Datei /u/ibmuser/config.txt auf SYSA
```

**Neues Verzeichnis anlegen**
```
Erstelle das Verzeichnis /u/ibmuser/projekte auf SYSA mit Berechtigungen rwxr-xr-x
```

**Datei löschen**
```
Lösche die Datei /u/ibmuser/old.log auf SYSA
```

---

### TSO

**Dataset-Katalog abfragen**
```
Führe auf SYSA den TSO-Befehl LISTCAT LVL(USER) aus
```

**Dataset-Details anzeigen**
```
Zeige mir mit TSO auf SYSA die Details des Datasets USER.JCL
```
*(entspricht `LISTDS USER.JCL`)*

**RACF-Berechtigungen prüfen**
```
Prüfe mit TSO auf SYSA die RACF-Berechtigungen für Dataset USER.SECURE.DATA
```
*(entspricht `LISTDSD DA(USER.SECURE.DATA) ALL`)*

---

### Parmlib

#### Alle aktiven BPXPRM-Members validieren

```
Validiere die Syntax aller aktiven BPXPRM Parmlib-Members auf SYSA
```

#### Einen bestimmten Member validieren

```
Validiere die Syntax von SYS1.PARMLIB(IEASYS00) auf SYSA
```

#### Alle Parmlib-Typen via LOADxx validieren

```
Validiere alle Parmlib-Typen mit LOADxx-Member LOAD00 aus SYS1.PARMLIB auf SYSA
```

#### Inline-Inhalt validieren

```
Validiere folgenden BPXPRM Parmlib-Inhalt auf SYSA: [Parmlib-Text...]
```

---

## 7. Automatisierungen erstellen

Der zosmf-mcp Server lässt sich mit Claude Code für komplexe Automatisierungen nutzen.

### Einfache Automatisierung: Job-Monitor

Prompt:
```
Überwache auf SYSA alle 5 Minuten alle aktiven Jobs und informiere mich,
wenn ein Job mit einem Return Code > 0 abgeschlossen wird.
Erstelle dazu ein Shell-Script monitor.sh das diese Prüfung durchführt.
```

### Komplexe Automatisierung: Patch-Installation prüfen

Prompt:
```
Prüfe auf SYSA ob folgende SYSMODs installiert sind: UA12345, UA12346, UA12347.
Erstelle eine Tabelle mit Status (installiert/nicht installiert),
Installations-Datum und dem jeweiligen FMID.
```

### Deployment-Pipeline

Prompt:
```
Führe folgende Schritte auf SYSA aus:
1. Lese das JCL-Template aus SYS1.PROCLIB(DEPLOY)
2. Ersetze &VERSION mit "V2R1" und &DATE mit dem heutigen Datum
3. Subitte den modifizierten Job ein
4. Warte auf Abschluss und zeige mir den Return Code
5. Falls RC > 0, zeige mir die JESMSGLG Spool-Datei
```

### System-Health-Check

Prompt:
```
Führe einen vollständigen Health-Check auf SYSA durch:
1. Systemzeit und Datum
2. Alle aktiven Address Spaces
3. Gemountete Filesysteme
4. OPERLOG der letzten 5 Minuten auf Fehlermeldungen
Fasse das Ergebnis als Bericht zusammen.
```

### Mehrere Systeme gleichzeitig

Wenn mehrere Systeme konfiguriert sind, können diese in einem Prompt verglichen werden:

```
Vergleiche die installierten Versionen von SYSMOD HBB77E0 auf SYSA und SYSB.
Sind beide Systeme auf dem gleichen Stand?
```

### Claude Code Hooks für Automatisierungen

Mit Claude Code Hooks können Aktionen automatisch ausgelöst werden. Beispiel `.claude/settings.json`:

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

### Geplante Aufgaben mit Claude Code

Mit dem `/schedule`-Befehl in Claude Code können wiederkehrende Aufgaben geplant werden:

```
/schedule täglich um 08:00 Uhr:
Prüfe auf SYSA ob alle kritischen Started Tasks (TCPIP, VTAM, JES2) aktiv sind.
Falls einer fehlt, erstelle eine Warnung.
```

---

## Weitere Hinweise

- **TLS**: Für Produktionssysteme sollte `skip_tls_verify: false` gesetzt und die CA-Zertifikate korrekt konfiguriert sein.
- **Berechtigungen**: Der konfigurierte z/OSMF-User benötigt entsprechende SAF/RACF-Berechtigungen für die verwendeten z/OSMF REST APIs.
- **TSO**: Für `zosmf_tso_command` muss `tso_account` und `tso_proc` in der Konfiguration gesetzt sein.
- **SMP/E**: Ohne explizite Angabe von `csi_dataset` und `zone` werden die Standardwerte aus der Konfiguration verwendet.
- **Passwörter**: Nie Passwörter in die Konfigurationsdatei hardcoden. Immer Umgebungsvariablen (`${VAR}`) verwenden.
