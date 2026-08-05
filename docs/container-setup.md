# zosmf-mcp im Container betreiben

Anleitung zum Aufsetzen von zosmf-mcp in einem Container, wenn der Rechner,
auf dem der Container läuft, die z/OSMF-Systeme **nicht direkt erreicht** —
sondern nur über einen Zwischenrechner mit SSH-Zugang.

Zwei Wege werden beschrieben: **Docker/Podman mit Compose** (Linux, Windows,
macOS) und **Apple container** (macOS auf Apple Silicon). Beide Wege wurden
gegen ein echtes z/OSMF durchgespielt.

---

## 1. Topologie

```text
Agent (n8n, Claude Code, …)
      │  HTTP + Bearer-Token
      ▼
┌─────────────────────────────────────────┐
│ Container-Host                          │
│                                         │
│  ┌───────────┐        ┌───────────────┐ │
│  │ zosmf-mcp │───────▶│ tunnel        │ │
│  │  :8081    │ SOCKS5 │  ssh -N -D    │ │
│  └───────────┘  :1080 └───────┬───────┘ │
└─────────────────────────────────┼───────┘
                                  │ SSH
                                  ▼
                          ┌───────────────┐
                          │ Jump-Host     │  nur sshd,
                          │ (sshd)        │  nichts zu installieren
                          └───────┬───────┘
                                  │
                                  ▼
                          z/OSMF (HTTPS :443)
```

Zwei Container. Der `tunnel`-Container baut per `ssh -N -D` einen
SOCKS5-Proxy auf und hält ihn offen. Der `zosmf-mcp`-Container schickt seine
z/OSMF-REST-Aufrufe durch diesen Proxy. Auf dem Jump-Host wird nichts
installiert — es genügt ein sshd und ein autorisierter Schlüssel.

Der SOCKS-Port 1080 wird **nie** nach außen veröffentlicht. Nur Port 8081 des
MCP-Servers ist erreichbar, und der verlangt einen Bearer-Token.

TLS bleibt Ende-zu-Ende: SOCKS5 tunnelt die TCP-Verbindung, die
TLS-Aushandlung findet zwischen Container und z/OSMF statt. Der Jump-Host
sieht nur verschlüsselten Verkehr.

---

## 2. Voraussetzungen

**Auf dem Jump-Host:** ein laufender sshd und ein autorisierter öffentlicher
Schlüssel. Sonst nichts.

**Auf dem Container-Host:** Docker mit Compose-Plugin, oder Apple container
1.2+ auf macOS 26 (Apple Silicon).

**Netzwerk:** Prüfe vor allem anderen, dass der Jump-Host die z/OSMF-Systeme
erreicht:

```bash
ssh <user>@<jump-host> 'nc -z -w 3 <zosmf-host> 443 && echo OK || echo FAIL'
```

Kommt hier kein `OK`, hilft der Rest der Anleitung nicht weiter.

---

## 3. Konfigurationsdatei

Der Container mountet deine **bestehende** Konfiguration. Es gibt keine
separate Container-Konfiguration und du musst deine Produktivdatei nicht
umschreiben.

Zwei Dinge sind wichtig:

**Der Transport wird beim Start überschrieben.** Deine Datei enthält
typischerweise `transport: stdio: true`. Im Container hängt nichts an stdin —
der Server bekäme sofort EOF und würde in einer Neustartschleife landen.
Deshalb wird der Transport per Kommandozeile erzwungen; die Compose-Datei und
die Make-Targets tun das bereits:

```
--transport streamable-http
```

**Der Proxy wird über die Umgebung aktiviert.** Du brauchst keinen
`proxy:`-Block in der Datei. Die Variable `ZOSMF_PROXY` setzt URL und schaltet
den Proxy ein. Ein `proxy:`-Block in der Datei hat Vorrang, und ein
`proxy: {enabled: false}` an einem einzelnen System bleibt eine wirksame
Ausnahme.

**Passwörter:** Referenzen wie `${ZOSMF_SYSA_PASSWORD}` werden aus der
Umgebung des Containers aufgelöst. Ist eine referenzierte Variable nicht
gesetzt, **startet der Server nicht** und nennt die fehlenden Namen. Das ist
Absicht — früher blieb der Platzhalter als Text stehen und äußerte sich viel
später als unerklärlicher 401.

---

## 4. `.env` anlegen

```bash
cp .env.example .env
```

Ausfüllen. Die Datei ist in `.gitignore`.

```bash
JUMP_HOST=user@host.example.com
JUMP_KEY=./secrets/jump_key
ZOSMF_CONFIG=/home/ronny/.config/smpe_ls/.smpe-zosmf.yaml
SSH_KEY_DIR=./secrets
MCP_API_KEY=<Ausgabe von: make apikey>
ZOSMF_SYSA_PASSWORD=…
ZOSMF_SYSB_PASSWORD=…
```

Die Passwort-Variablennamen müssen **exakt** zu den `${...}`-Referenzen in
deiner Konfigurationsdatei passen.

> **Nur `KEY=value`, kein `export` davor.** Sowohl `docker compose`
> (`env_file`) als auch `container run --env-file` lehnen die export-Form ab.
> Apple container meldet dann:
> `Error: variable 'export ZOSMF_SYSA_PASSWORD' contains whitespaces`

API-Key erzeugen:

```bash
make apikey          # openssl rand -base64 32
```

---

## 5. SSH-Schlüssel bereitlegen

Erzeuge einen **eigenen** Schlüssel nur für den Tunnel. Nimm nicht deinen
persönlichen Alltagsschlüssel — der Container-Schlüssel liegt unverschlüsselt
auf dem Host und ist nur für genau diesen einen Jump-Host autorisiert.

```bash
mkdir -p secrets
ssh-keygen -t ed25519 -N '' -f secrets/jump_key -C 'zosmf-mcp tunnel'
ssh-copy-id -i secrets/jump_key.pub <user>@<jump-host>
```

**Unter Linux zusätzlich nötig:** Der Tunnel-Container läuft als root und
sieht die echte Host-UID der gemounteten Datei. Gehört sie deinem Benutzer,
lehnt OpenSSH sie ab und der Container läuft in eine Neustartschleife.

```bash
sudo chown 0:0 secrets/jump_key
sudo chmod 600 secrets/jump_key
```

Unter macOS ist das nicht nötig — die Dateisystem-Virtualisierung bildet die
Eigentümerschaft passend ab.

---

## 6. Weg A: Docker mit Compose

### Bauen und starten

```bash
make docker-build     # docker compose build
make docker-up        # docker compose up -d
make docker-logs      # docker compose logs -f
make docker-down      # docker compose down
```

Oder ohne Make:

```bash
docker compose build
docker compose up -d
docker compose logs -f zosmf-mcp
```

### Was die Compose-Datei tut

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

Der Servicename `tunnel` löst im Compose-Netz per DNS auf, deshalb steht er
direkt in `ZOSMF_PROXY`. `env_file` reicht **alle** Variablen durch,
einschließlich der systembezogenen Passwörter.

`depends_on` wartet nur auf den Start des Tunnels, nicht auf dessen
Bereitschaft. Der Tunnel steht typischerweise in unter einer Sekunde; ein
Agent, der sich erst später verbindet, merkt davon nichts.

---

## 7. Weg B: Apple container

Apple container kennt kein Compose. Die beiden Container werden einzeln
gestartet, und es gibt **keine Namensauflösung zwischen Containern** ohne
registrierte DNS-Domain — die IP des Tunnels wird deshalb zur Laufzeit
ermittelt.

### Einmalig: Rosetta-Anforderung abschalten

Apples BuildKit-Builder startet in einer VM, die standardmäßig Rosetta
einbindet, um auch `linux/amd64` bauen zu können. Ohne Rosetta scheitert
bereits der Start des Builders:

```
Error: failed to bootstrap container buildkit … "Rosetta is not installed"
```

Unser Image ist reines arm64. Statt Rosetta zu installieren, schaltest du die
Anforderung ab:

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

Die Datei wird mit den Standardwerten zusammengeführt; alle übrigen
Einstellungen bleiben unverändert.

### Bauen und starten

```bash
make container-build   # container build --platform linux/arm64 -t zosmf-mcp:latest .
make container-up      # Tunnel + Server, IP-Auflösung inklusive
make container-logs    # container logs -f zosmf-mcp
make container-down    # beide Container stoppen und entfernen
```

### Dieselben Schritte ohne Make

```bash
# 1) Builder starten und Image bauen
container builder start
container build --platform linux/arm64 -t zosmf-mcp:latest .

# 2) Tunnel starten
container run -d --name zosmf-tunnel \
  -v "$PWD/secrets/jump_key:/key" \
  --entrypoint /usr/bin/ssh zosmf-mcp:latest \
  -N -D 0.0.0.0:1080 -i /key \
  -o ExitOnForwardFailure=yes -o ServerAliveInterval=30 \
  -o ServerAliveCountMax=3 -o StrictHostKeyChecking=accept-new \
  -o BatchMode=yes user@host.example.com

# 3) IP des Tunnels ermitteln
TUNNEL_IP=$(container ls | awk '/^zosmf-tunnel/{print $6}' | cut -d/ -f1)
echo "$TUNNEL_IP"

# 4) .env für --env-file aufbereiten (entfernt ein evtl. vorhandenes "export")
sed -E 's/^[[:space:]]*export[[:space:]]+//' .env \
  | grep -E '^[A-Za-z_][A-Za-z0-9_]*=' > /tmp/zosmf-env
chmod 600 /tmp/zosmf-env

# 5) MCP-Server starten
container run -d --name zosmf-mcp \
  -v "$HOME/.config/smpe_ls/.smpe-zosmf.yaml:/config/.smpe-zosmf.yaml" \
  -v "$PWD/secrets:/keys" \
  --env-file /tmp/zosmf-env \
  -e ZOSMF_PROXY=socks5://$TUNNEL_IP:1080 \
  -p 8081:8081 \
  zosmf-mcp:latest --transport streamable-http

rm -f /tmp/zosmf-env
```

Für das eigene Image als Tunnel gibt es einen Grund: `alpine/openssh` ließ
sich hier nicht anonym von Docker Hub ziehen (`401 Unauthorized`), und unser
Image enthält ohnehin `openssh-client`. Unter Docker funktioniert
`alpine/openssh` normal.

---

## 8. Prüfen, ob alles läuft

```bash
# Container laufen?
docker compose ps            # bzw.: container ls

# Tunnel steht?
docker compose logs tunnel   # bzw.: container logs zosmf-tunnel
# Erwartet: keine Fehler. "Permanently added … to the list of known hosts"
# beim ersten Start ist normal.

# Server lauscht und verlangt Authentifizierung?
curl -s -o /dev/null -w '%{http_code}\n' -X POST http://localhost:8081/mcp \
  -H 'Content-Type: application/json' -d '{}'
# Erwartet: 401
```

Vollständiger Durchstich mit einem echten Tool-Aufruf:

```bash
KEY=<dein MCP_API_KEY>

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

Kommt hier eine Datasetliste zurück, funktioniert die gesamte Kette:
HTTP-Auth, Proxy, Tunnel, Jump-Host, z/OSMF.

---

## 9. Client anbinden

Endpunkt: `http://<container-host>:8081/mcp`
Header: `Authorization: Bearer <MCP_API_KEY>`

Für den n8n MCP Client Tool Node (ab v1.88) trägst du die URL als Server-URL
ein und den Bearer-Token als Header-Auth.

Da der Token über das Netz geht, gehört dieses Setup in ein vertrauenswürdiges
Netz. Willst du es darüber hinaus erreichbar machen, setze einen Reverse-Proxy
mit TLS davor — der Server selbst spricht kein HTTPS.

---

## 10. Wenn etwas nicht funktioniert

**`unresolved environment variables in …: ZOSMF_SYSA_PASSWORD`**
Die Konfiguration referenziert eine Variable, die im Container nicht ankommt.
Prüfe, dass der Name in `.env` exakt passt und dass `env_file` bzw.
`--env-file` gesetzt ist. Unter Apple container: enthält deine `.env` noch
`export`?

**`transport streamable_http: api_key is required`**
`MCP_API_KEY` ist leer. Mit `make apikey` erzeugen und in `.env` eintragen.
Bewusst ohne Authentifizierung starten geht nur mit `allow_insecure: true` im
`transport`-Block der Konfiguration — nur sinnvoll, wenn Port 8081
nachweislich nicht erreichbar ist.

**Tool-Aufrufe laufen ~60 Sekunden und melden `context deadline exceeded`**
Der Verkehr geht am Tunnel vorbei. Prüfe, dass `ZOSMF_PROXY` im Container
gesetzt ist und auf den Tunnel zeigt. Unter Apple container: hat sich die
Tunnel-IP nach einem Neustart geändert? Dann `make container-down` und
`make container-up`.

**Tunnel-Container startet immer wieder neu**
Fast immer der Schlüssel. Unter Linux muss er root gehören und Modus 600
haben. `docker compose logs tunnel` zeigt die Meldung von OpenSSH.

**`401` von z/OSMF trotz gesetzter Variablen**
Echte Zugangsdaten falsch, oder der Benutzer ist auf dem Zielsystem gesperrt.
Seit der Startprüfung auf unaufgelöste Variablen ist eine fehlende
Umgebungsvariable als Ursache ausgeschlossen — der Server wäre gar nicht erst
gestartet.

**Apple container: `HTTP 000` auf `localhost:8081` direkt nach dem Start**
Die Portfreigabe braucht einen Moment. Ein paar Sekunden warten und erneut
versuchen; alternativ direkt gegen die Container-IP aus `container ls`.
