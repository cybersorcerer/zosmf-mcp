# Einheitliche Konfigurationsdatei `.smpe-zosmf.yaml` — Design

**Datum:** 2026-07-11
**Status:** Approved

## Zusammenfassung

Vier Komponenten brauchen eine z/OSMF-Konfiguration: smpe_ls (via vscode-smpe-Client), zosmf-mcp, vscpdf, nvpdf.nvim. Heute existieren zwei Dateinamen: `.smpe-zosmf.yaml` (drei Komponenten) und `.zosmf-mcp-config.yaml` (nur als CLI-Flag-Default von zosmf-mcp plus README). Ziel: **nur noch `.smpe-zosmf.yaml`**, kanonischer Listen-Schlüssel **`servers:`**, Schema additiv um `auth`/`tso_*`/`ssh_systems` erweitert — vollständig abwärtskompatibel für die ~20 Bestandsinstallationen von smpe_ls.

Kernbefund der Analyse: die Konsolidierung ist im Schema bereits angelegt. zosmf-mcp akzeptiert `servers:` als Alias für `systems:` und mappt die smpe_ls-Felder (`user` → `auth.username`, `rejectUnauthorized` → `skip_tls_verify`, `port` → Host-Suffix, `csi`/`zones`/`defaultZones`). Die einzige Code-Änderung ist die Default-Suchlogik in zosmf-mcp; alles andere ist Doku und Templates.

## Bestandsaufnahme

| Komponente | Datei heute | Schlüssel | liest die Datei |
| --- | --- | --- | --- |
| vscode-smpe (smpe_ls/client) | `.smpe-zosmf.yaml` | `servers:` | TS-configManager; der Go-LS selbst nie |
| vscpdf | `.smpe-zosmf.yaml` | `systems:` oder `servers:` | ja; übergibt Pfad explizit an zosmf-mcp |
| zosmf-mcp | `--config <pfad>`, Default `.zosmf-mcp-config.yaml` | `systems:` mit `servers:`-Alias, volle Feldmenge | ja |
| nvpdf.nvim | `.smpe-zosmf.yaml` (cwd → `~/.config/smpe_ls/`) | `servers:` | ja |

## Änderung 1: zosmf-mcp Default-Suchlogik (einzige Code-Änderung)

`cmd/zosmf-mcp/main.go`: Flag-Default von `.zosmf-mcp-config.yaml` auf `""`. Bei leerem `--config` sucht der Server in dieser Reihenfolge:

1. `./.smpe-zosmf.yaml` (Arbeitsverzeichnis)
2. `~/.config/smpe_ls/.smpe-zosmf.yaml`
3. `./.zosmf-mcp-config.yaml` — Legacy; startet normal, loggt aber auf stderr:
   `[zosmf-mcp] WARNING: .zosmf-mcp-config.yaml is deprecated — rename to .smpe-zosmf.yaml`
4. Nichts gefunden → Fehler mit Auflistung der drei geprüften Pfade.

Explizit gesetztes `--config <pfad>` verhält sich exakt wie heute (vscpdf und andere Clients übergeben den Pfad ohnehin explizit — für sie ändert sich nichts).

## Änderung 2: Randfall `servers:`-Eintrag ohne `user`

`validate()` lehnt heute Einträge ab, deren `auth.Type` leer bleibt (kein `user`, kein `auth`, kein Zertifikat) — eine alte smpe_ls-Datei ohne `user:`-Feld würde den **Serverstart** verhindern. Abfederung: solche Einträge werden beim Laden toleriert (Log-Hinweis) und wie „ohne Credentials" registriert; jeder Tool-Call darauf liefert den bestehenden Marker `NO_CREDENTIALS(<SYSTEM>): no username configured — add 'user:' to the server entry`. Damit blockiert ein unvollständiger Alt-Eintrag nie die übrigen Systeme. (Betrifft nur zosmf-mcp-Neueinsteiger; bestehende smpe_ls-Installationen laufen unabhängig davon unverändert.)

## Änderung 3: Kanonisches Schema dokumentieren (zosmf-mcp README = Source of Truth)

`servers:`-Einträge mit: `name`, `host`, `port`, `user`, `rejectUnauthorized`, `csi`, `zones`, `defaultZones` (bestehende smpe_ls-Felder) **plus optional** `auth:` (`username`, `password` — inkl. `${ENV_VAR}`-Expansion), `tso_account`, `tso_proc`, `tso_charset`, `tso_codepage`, `ca_file`, `home`. Top-Level optional: `ssh_systems:`, `transport:`, `defaultServer` (von vscode-smpe genutzt, von zosmf-mcp ignoriert). README-Beispiele von `systems:` auf `servers:` umstellen; ein Vollbeispiel mit allen optionalen Feldern auskommentiert.

## Änderung 4: Templates/Doku der Clients (keine Code-Änderungen)

- **vscode-smpe** (`client/vscode-smpe/src/zosmf/configManager.ts`, eingebettetes Template): optionale Felder als Kommentare ergänzen (`auth:`, `tso_account`, `ssh_systems:`), Hinweis „shared with zosmf-mcp/vscpdf/nvpdf".
- **nvpdf.nvim** (`lua/nvpdf/template.lua`): dito.
- **vscpdf** (README, CLAUDE.md): Beispiel von `systems:` auf `servers:` vereinheitlichen; kein Code.

## Abwärtskompatibilität (20 smpe_ls-Kundeninstallationen)

- An vscode-smpe/nvpdf/smpe_ls wird **kein Code** geändert; Templates betreffen nur neu generierte Dateien.
- Das Schema wächst nur **additiv**; bestehende Dateien bleiben byte-identisch gültig. Unbekannte neue Felder werden von den TS-/Lua-Parsern ignoriert.
- zosmf-mcp liest Alt-Dateien über die vorhandenen Aliase korrekt; fehlende TSO-Felder wirken sich nur auf `zosmf_tso_command` aus (klare Fehlermeldung, wie heute).
- `.zosmf-mcp-config.yaml` funktioniert übergangsweise weiter (Deprecation-Warnung), Entfernung frühestens nach einer Major-Version.

## Nicht-Ziele

- Kein neuer Dateiname (`.smpe-zosmf.yaml` bleibt kanonisch).
- Kein `systems:`-Support in vscode-smpe/nvpdf (kanonisch ist `servers:`; zosmf-mcp/vscpdf behalten ihre Alias-Toleranz).
- Keine Schema-Validierung/Migrationstooling.

## Verifikation

1. zosmf-mcp Unit-Tests: Suchreihenfolge (TempDir: nur cwd-Datei / nur Home-Datei / nur Legacy → Warnung / nichts → Fehler mit drei Pfaden); `validate()`-Toleranz für Eintrag ohne `user` + `NO_CREDENTIALS`-Marker beim Tool-Call.
2. `make all` in zosmf_mcp grün.
3. Manuell: `zosmf-mcp` ohne `--config` neben einer `.smpe-zosmf.yaml` starten → Systeme registriert; mit nur `.zosmf-mcp-config.yaml` → Start + Warnung.
4. Bestands-Check: eine unveränderte Kunden-`.smpe-zosmf.yaml` (nur smpe_ls-Felder) gegen zosmf-mcp → Start ok, Dataset/Job-Tools funktionieren, TSO meldet fehlendes `tso_account`.
5. vscpdf/nvpdf/vscode-smpe: unverändert lauffähig (keine Code-Änderung, Stichprobe im Extension Development Host bzw. Neovim).
