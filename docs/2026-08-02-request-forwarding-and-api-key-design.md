# Request-Forwarding und API-Key-Absicherung

Datum: 2026-08-02
Status: Entwurf, freigegeben zur Planung

## Problem

Der zosmf-mcp-Server läuft heute auf einer Maschine mit direktem Netzzugang zu den
z/OS-Systemen ("Maschine 2"). AI-Agents müssen ihre MCP-Requests dorthin schicken.
Das koppelt den Ort, an dem der Server läuft, an den Ort, von dem aus die z/OS-Systeme
erreichbar sind.

Ziel ist die Umkehrung: Der Server läuft in Docker im Homelab, und nur die
z/OSMF-REST-Calls (sowie die ZOAU-SSH-Verbindungen) werden über Maschine 2 geleitet.
Ob dieses Forwarding stattfindet, muss vollständig konfigurierbar sein — ohne
Forwarding verhält sich der Server exakt wie heute.

## Topologie

```text
MacBook (privat)          Homelab-Dockerhost           Maschine 2                z/OS
──────────────────        ────────────────────         ──────────────            ──────
Claude Code / Agent  ──▶  zosmf-mcp ──▶ tunnel   ──▶   sshd (Jump)   ──▶  z/OSMF REST
                 (MCP über HTTP)   (SOCKS5)      (SSH)              (HTTPS)
```

Der Tunnel-Sidecar läuft im Homelab neben dem mcp-Container und baut eine ausgehende
SSH-Verbindung zu Maschine 2 auf. Auf Maschine 2 wird nichts installiert — `sshd`
genügt, `ssh -D` wird vollständig vom lokalen Client bedient.

Der Dockerhost erreicht Maschine 2 direkt per SSH (kein VPN nötig). Der Agent auf dem
MacBook erreicht den Server über das LAN.

## Warum diese Architektur

Alle z/OSMF-REST-Calls laufen durch genau einen `http.Client`:

```text
tools/*.go → zosmf/{jobs,datasets,console,tso,parmlib,smpe,ussfiles,filesystems}.go
           → http.NewRequest(...)   (18 Stellen, nur Request-Bau)
           → c.Do(req)              client.go:121
           → c.httpClient.Do(req)   client.go:146   ← einziger Netzwerk-Ausgang
```

`c.httpClient` wird einmal in `NewClient` gebaut (`client.go:92-97`). Ein Proxy an dieser
Stelle deckt daher 100 % der REST-Funktionalität ab, ohne einen einzigen Tool-Handler
oder eine der 18 Request-Stellen anzufassen.

Gos `http.Transport.Proxy` unterstützt `http`, `https` und `socks5` und nutzt bei
HTTPS-Zielen automatisch CONNECT-Tunneling. Die TLS-Session endet weiterhin im
Container gegen z/OSMF — Maschine 2 sieht nur verschlüsselte Bytes, es gibt keine
Zertifikatsprobleme und `skip_tls_verify` bleibt unnötig. Gos SOCKS5-Dialer schickt
zudem den Hostnamen an den Proxy statt lokal aufzulösen, sodass die z/OS-Namen auf
Maschine 2 aufgelöst werden.

### Verworfene Alternativen

- **HTTP-CONNECT-Proxy (squid/tinyproxy) auf Maschine 2** — erfordert Installation und
  Betrieb zusätzlicher Software auf einer Maschine, die nicht uns gehört.
- **SSH-Tunnel pro System (`-L 10443:zos1:443`)** — ändert die Zielhostnamen, bricht die
  TLS-Zertifikatsprüfung und würde `skip_tls_verify` erzwingen.
- **ssh-Subprozess im mcp-Container** — verlagert Prozess-Supervision, Reconnect-Loop
  und Startup-Gating in Go (~150 Zeilen), die Docker per `restart: unless-stopped`
  gratis erledigt.
- **Nativer SSH-Tunnel via `golang.org/x/crypto/ssh`** — technisch elegant (kein SOCKS,
  kein offener Port), kostet aber neue Dependencies und bricht mit dem exec-Stil in
  `ssh.go`.

## Konfigurationsschema

```yaml
# global: gilt für alle Systeme
proxy:
  enabled: true
  url: socks5://tunnel:1080          # oder ${ZOSMF_PROXY}

ssh_proxy_jump:                       # für ZOAU über Maschine 2
  enabled: true
  host: jump@jumphost.example.com

transport:
  streamable_http:
    enabled: true
    listen: "0.0.0.0:8081"
    api_key: ${MCP_API_KEY}
    # allow_insecure: true            # nur nötig, um ohne api_key zu starten

servers:
  - name: SYS1
    host: https://zos1.example.com:443
    user: IBMUSER
  - name: SYS2
    host: https://zosmf-internal.example.com:443
    proxy:
      enabled: false                  # dieses System direkt, ohne Tunnel

ssh_systems:
  - name: SYS1
    host: zos1.example.com
    username: ibmuser
    key_file: /keys/zos1
```

Regeln:

- Fehlender Block oder `enabled: false` = heutiges Verhalten, exakt unverändert.
- Per-System schlägt global.
- `ZOSMF_PROXY` füllt `proxy.url`, wenn dort nichts steht.
- Erlaubte Schemas: `http`, `https`, `socks5`. Alles andere wird beim Laden abgelehnt,
  damit Tippfehler nicht erst beim ersten Tool-Call auffallen.
- `proxy` und `ssh_proxy_jump` sind unabhängig voneinander schaltbar.

## Code-Änderungen

| Datei | Änderung |
|---|---|
| `internal/config/config.go` | `ProxyConfig`/`SSHProxyJumpConfig` Structs; Felder in `Config`, `SystemConfig`, `SSHSystemConfig`; Vererbung global→System in `normalize()`; URL- und Schema-Prüfung in `validate()`; `APIKey` in `HTTPConfig` |
| `internal/zosmf/client.go:92` | `transport.Proxy = http.ProxyURL(u)`, wenn aktiv |
| `internal/zosmf/ssh.go:36` | `-J <host>` an die args, wenn aktiv |
| `internal/mcp/transport_http.go:42,53` | `NewHTTPTransport` nimmt API-Key; `mux` in `requireAPIKey`-Middleware wickeln |
| `cmd/zosmf-mcp/main.go:163,183` | zwei Call-Sites anpassen; Startabbruch bei HTTP-Transport ohne Key |
| `Dockerfile`, `docker-compose.yml`, `.dockerignore` | neu |
| `Makefile` | Targets `docker-build`, `docker-up`, `apikey` |

`Client` bekommt kein neues Feld — der Proxy sitzt im `http.Transport`, wo Tests ihn
über `Proxy(req)` verifizieren können.

### API-Key-Middleware

```go
func requireAPIKey(key string, next http.Handler) http.Handler {
	if key == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="zosmf-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

`subtle.ConstantTimeCompare` statt `==`, damit die Antwortzeit den Key nicht zeichenweise
verrät. Leerer Key = Middleware inaktiv, wodurch stdio-Nutzer unberührt bleiben.

Beide Transports (`http` und `streamable_http`) nutzen dieselbe `NewHTTPTransport` und
registrieren genau einen Handler auf `/mcp`, sodass ein Middleware-Wrap beide abdeckt.
Jeder Transport hat einen eigenen `HTTPConfig`-Block und damit einen eigenen Key.

Der Serverstart bricht ab, wenn ein HTTP-Transport aktiv ist und kein `api_key` gesetzt
wurde. Bewusst unsicherer Betrieb erfordert ein explizites `allow_insecure: true`.

## Docker-Setup

```yaml
services:
  tunnel:
    image: alpine/openssh
    command: >
      ssh -N -D 0.0.0.0:1080 -i /key
      -o ExitOnForwardFailure=yes -o ServerAliveInterval=30
      -o StrictHostKeyChecking=accept-new
      jump@jumphost.example.com
    volumes: [./secrets/jump_key:/key:ro]
    restart: unless-stopped

  zosmf-mcp:
    build: .
    depends_on: [tunnel]
    ports: ["8081:8081"]
    volumes:
      - ./.smpe-zosmf.yaml:/config/.smpe-zosmf.yaml:ro
      - ./secrets:/keys:ro
    environment:
      ZOSMF_PASSWORD: ${ZOSMF_PASSWORD}
      ZOSMF_PROXY: socks5://tunnel:1080
      MCP_API_KEY: ${MCP_API_KEY}
    restart: unless-stopped
```

Port 1080 wird nie nach außen gemappt, der SOCKS5-Endpunkt ist nur im Docker-internen
Netz sichtbar. Das Image braucht `openssh-client` für die ZOAU-Tools. Der Server muss
auf `0.0.0.0:8081` binden, sonst greift das Port-Mapping nicht.

### API-Key erzeugen und verteilen

```bash
make apikey        # openssl rand -base64 32
```

Auf dem Dockerhost in `.env` neben `docker-compose.yml` (bereits in `.gitignore`):

```bash
MCP_API_KEY=…
ZOSMF_PASSWORD=…
```

Auf dem MacBook als Header in der MCP-Client-Konfiguration:

```json
{
  "mcpServers": {
    "zosmf": {
      "type": "http",
      "url": "http://dockerhost:8081/mcp",
      "headers": { "Authorization": "Bearer …" }
    }
  }
}
```

Rotation: neuen Wert erzeugen, `.env` ändern, `docker compose up -d zosmf-mcp`,
Client-Config nachziehen.

## Tests

- **config**: Proxy-Vererbung global→System; `enabled: false` pro System überschreibt
  aktives Global; `ZOSMF_PROXY` füllt leeres `url`; ungültiges Schema wird abgelehnt;
  fehlender Block ergibt inaktives Forwarding.
- **client**: `NewClient` mit aktivem Proxy setzt `Transport.Proxy`, verifiziert über
  `Proxy(req)`; ohne Proxy bleibt das Feld `nil`.
- **ssh**: Argumentliste enthält `-J` genau dann, wenn `ssh_proxy_jump` aktiv ist.
- **transport_http**: Request ohne Header → 401; falscher Key → 401; korrekter Key →
  durchgereicht; leerer konfigurierter Key → Middleware inaktiv.

## Bekannte Risiken

- **Klartext-HTTP im LAN.** Ein API-Key über unverschlüsseltes HTTP schützt gegen ein
  Gerät, das den Port zufällig findet oder scannt, aber nicht gegen jemanden, der den
  Verkehr mitlesen kann: Key und alle z/OS-Kommandos gehen im Klartext über das Netz.
  Für ein Heimnetz mit vertrauenswürdigen Geräten ein bewusst akzeptierter Kompromiss.
  Nächster Schritt wäre TLS, direkt im Transport oder über einen Reverse-Proxy.
- **`depends_on` wartet nur auf Container-Start**, nicht auf einen stehenden Tunnel. Die
  ersten Requests nach einem Neustart können fehlschlagen, bis die SSH-Verbindung steht.
- **Secrets im Homelab.** Auf dem Dockerhost liegen der SSH-Key für Maschine 2 und die
  z/OSMF-Credentials.
- **`.smpe-zosmf.yaml` ist nicht in `.gitignore`** (nur die alte
  `.zosmf-mcp-config.yaml`). Vor der Umsetzung zu klären, ob sie ergänzt werden soll.
- **n8n-MCP-Client-Node**: ob dort beliebige Header gesetzt werden können, ist ungeprüft.
  Ein Key als Query-Parameter wäre keine Alternative, weil er in Logs landet.

## Nicht im Scope

- TLS für die MCP-Transports.
- Mehrere gleichzeitig gültige API-Keys oder Key-Rotation ohne Neustart.
- Änderungen an den Tool-Handlern in `internal/tools/`.
