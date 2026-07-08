# Per-System-Credentials Design (Server-Seite)

**Datum:** 2026-07-08
**Status:** Approved
**Branch:** `feat/per-system-credentials`
**Gegenstück:** `vscpdf` Repo, `docs/superpowers/specs/2026-07-08-per-system-credentials-design.md` (Extension-Seite)

## Zusammenfassung

Systeme in der Config können unterschiedliche RACF-Datenbanken haben — eine UserID kann pro System unterschiedliche Passwörter haben. Heute kopiert `normalize()` das eine `ZOSMF_PASSWORD` env in alle Systeme ohne eigenes Passwort. Dieses Design ergänzt: ein MCP-Tool `zosmf_set_password`, mit dem ein Client (vscpdf-Extension) Passwörter pro System zur Laufzeit in den laufenden Prozess pusht, plus definierte Fehlermarker, damit der Client fehlendes/falsches Passwort unterscheiden und gezielt reagieren kann.

---

## Neues Tool `zosmf_set_password`

- Argumente: `{ system: string, password: string }` — beide required. Username kommt weiterhin aus der Config (`auth.username`).
- Wirkung: setzt das Passwort des registrierten `zosmf.Client` für `system` in-memory (Setter mit Mutex, da Tool-Calls nebenläufig zu laufenden Requests kommen können). Nie auf Platte, nie in Logs.
- Unbekanntes System: Fehler `unknown system: <name>` (bestehendes Verhalten von `ClientRegistry.Get`).
- Registrierung in `internal/tools/registry.go` wie alle anderen Tools; Implementierung z.B. `internal/tools/auth.go`.

## Config-Änderung: Systeme ohne Passwort zulassen

- `normalize()`/`validate()` dürfen ein System ohne Passwort (weder YAML noch env-Fallback) nicht mehr ablehnen. Der Client wird trotzdem registriert, markiert als „ohne Credentials".
- Env-Fallback `ZOSMF_PASSWORD` bleibt unverändert: Systeme ohne eigenes Passwort erben es (Abwärtskompatibilität für alte Clients, die kein `zosmf_set_password` kennen).

## Fehlermarker

Tool-Fehler propagieren als Strings. Für den Client müssen zwei Fälle maschinell unterscheidbar sein — stabile Präfixe:

| Präfix | Wann |
|---|---|
| `NO_CREDENTIALS(<system>):` | Tool-Call auf ein System, dessen Client kein Passwort hat |
| `AUTH_FAILED(<system>):` | z/OSMF antwortet HTTP 401 auf einen Request für dieses System |

- `NO_CREDENTIALS` wird vor dem HTTP-Request geworfen (Client-seitig im Prozess), erzeugt keinen Fehlversuch in RACF.
- `AUTH_FAILED` erfordert, dass der HTTP-Client 401 erkennt und den Statuscode nicht in einer generischen Fehlermeldung verschluckt.
- Die Präfixe sind Schnittstellen-Vertrag mit der vscpdf-Extension — Format nicht ändern ohne beide Repos anzupassen.

## Nicht-Ziele

- Keine Persistenz gepushter Passwörter (Prozess-Restart = Client pusht neu).
- Keine Änderung an `ssh_systems`/ZOAU-Credentials.
- Kein Passwort-Feld in der YAML entfernen — `auth.password` und `${ENV_VAR}`-Expansion bleiben.

## Verifikation

1. `make build` grün, bestehende Tests grün.
2. Unit-Test: Config mit System ohne Passwort lädt; Tool-Call darauf liefert `NO_CREDENTIALS(<system>): ...`.
3. Unit-Test: `zosmf_set_password` gegen registriertes System, danach nutzt der Client das neue Passwort.
4. Unit-Test: 401-Antwort (httptest) liefert `AUTH_FAILED(<system>): ...`.
5. Manuell gegen echtes z/OSMF: falsches Passwort → `AUTH_FAILED`, korrektes Passwort per `zosmf_set_password` → Folge-Call erfolgreich.
