# Guided setup — design

Target release: v0.7.0

## The problem

Setting up `zosmf-mcp` today means writing YAML by hand. The reference is a
1400-line README and a commented example file. A new user has to know which
fields the SMP/E tools need, that passwords belong in the environment and not
in the file, that the configuration is shared with `smpe_ls`, and how to
register the server with their client. Every one of those is a place to get
it wrong quietly: a missing `csi` surfaces much later as
`no csi_dataset provided and system X has no csi_datasets configured`, and a
mistyped variable name surfaces as a 401 from z/OSMF.

An AI agent sitting in front of that documentation is not better off. It has
to read a lot of prose to produce one file, and two runs produce two
different files.

## Goals

- A setup that produces the same configuration whoever — or whatever — runs it.
- Usable by a human at a terminal, by Claude Code, and by agents running
  local models (opencode, pi) that cannot be trusted to author YAML.
- Never lose what other tools put in the shared configuration file.
- Never write a password into a configuration file.

## Non-goals

- Replacing the README. The setup covers the common path; exotic
  configurations stay hand-written.
- Certificate authentication in the wizard. It stays configurable by hand
  until someone asks for it.
- Managing the client configuration formats of other tools from Go — see
  "The agent skill".

## Overview

Two new subcommands in the binary do the work that must be deterministic. A
short, portable instruction file drives them and covers the steps the binary
cannot reach — registering with a client, starting a container.

```
zosmf-mcp init config     # create or update one system in the YAML
zosmf-mcp init env        # write .env: passwords and the API key
```

```
agents/setup.md           # the canonical procedure, one file
agents/claude/SKILL.md    # thin wrapper, Claude Code format
agents/opencode/…         # thin wrapper, opencode format
agents/pi/…               # thin wrapper, pi format
```

## 1. Command surface

Both subcommands share one behaviour matrix, decided without user input:

| Invocation | Behaviour |
| --- | --- |
| no flags, stdin is a TTY | interactive wizard, one question at a time |
| any field flag given | non-interactive, writes directly |
| no flags, no TTY | error naming `--questions` and the field flags; never blocks on a prompt nobody will answer |
| `--questions` | field catalog as JSON on stdout, writes nothing |

Shared switches:

- `--file <path>` — overrides the search path.
- `--print` — render the result to stdout, write nothing.
- `--check` — `init config` only: load the file through the server's own
  `config.Load` and report what it rejects. This gives the skill a
  verification step that cannot disagree with what the server will do at
  startup. For `init env` the equivalent check is that the file exists with
  mode 600 and defines every variable the configuration needs.

File discovery follows the server: `.smpe-zosmf.yaml` in the working
directory, then `~/.config/smpe_ls/.smpe-zosmf.yaml`, then the legacy name.
When nothing exists, `init config` proposes the second path, because that is
the file the other tools share.

`init env` reads the configuration first. It collects the systems and any
`${...}` references, then asks only for what is actually needed. It generates
`MCP_API_KEY` on request instead of sending the user to `make apikey`. The
file is written with mode 600, as plain `KEY=value` without `export`, because
both `docker compose --env-file` and `container run --env-file` reject the
export form.

## 2. The field catalog

One list in Go is the single source. The wizard prompts, the flag set and the
JSON output are all generated from it. A new field is added once.

Each entry carries: id, label, help text, type, cardinality, required,
default, example, group. Cardinality is what makes lists survive the trip
through an agent — the catalog states that `csi` takes several values instead
of leaving a model to guess.

### Group "system" — one invocation per LPAR

| Field | Required | Type | Note |
| --- | --- | --- | --- |
| `name` | yes | string | uppercased; used as the `system` argument in tool calls |
| `host` | yes | string | with scheme; port here or in `port` |
| `port` | no | int | appended to the host when the host carries none |
| `user` | yes | string | TSO user id, written in the `smpe_ls` spelling `user:` |
| `csi` | recommended | list | the SMP/E tools use the **first** entry as their default |
| `zones` | recommended | list | **all** entries are the default |
| `defaultZones` | no | list | read by vscode-smpe; the wizard offers to mirror `zones` |
| `tso_account` | no | string | `zosmf_tso_command` fails without it |
| `tso_proc` | no | string | as above |
| `rejectUnauthorized` | no | bool | `smpe_ls` spelling; the server maps it to `skip_tls_verify` |
| `ca_file` | no | path | the alternative to disabling verification |
| `home` | no | path | belongs to vscpdf and nvpdf; written only because we own the file |

`csi` and `zones` are formally optional and practically required: without
them every SMP/E tool call has to carry them explicitly, which an agent that
does not know the site's zone names cannot do. The wizard asks for them in
the normal flow and states the consequence when they are skipped.

### Group "ZOAU" — only when the user wants those tools

Asked: SSH host (default: the host part of `host`), user (default: `user`
lowercased), key file, and the **ZOAU installation path**. The last one
derives three variables, so the user answers one question instead of three:

```yaml
environment:
  ZOAU_HOME: "<path>"
  PATH:      "<path>/bin:/bin:/usr/bin"
  LIBPATH:   "<path>/lib:/lib:/usr/lib"
```

A user who needs a different layout edits the map by hand; a later run does
not touch it.

### Not a field: the password

`init config` never writes a password or a password reference. See section 4.

### Transport

Global, not per system. `init config` writes a transport block only when the
file has none; an existing one is left alone.

### Catalog shape

```json
{"command":"init config","fields":[
  {"id":"name","label":"System name","type":"string","cardinality":"one",
   "required":true,"example":"MAINT","group":"system",
   "help":"Name used as the 'system' argument in tool calls."},
  {"id":"csi","label":"CSI datasets","type":"string","cardinality":"many",
   "required":false,"example":["ZOS31.SMPE.GLOBAL.CSI"],"group":"system",
   "help":"SMP/E tools use the first entry when no csi_dataset is given."}
]}
```

## 3. Writing the file

The configuration file is shared with `smpe_ls`, vscode-smpe and nvpdf. It
contains keys this server does not know — `home`, `defaultServer` — and
comments its owner wrote. Loading it into Go structs and marshalling it back
would delete both.

`init config` therefore edits on the node level: load into a `yaml.Node`
tree, find the entry in the `servers:` sequence whose name matches
(case-insensitively, because the server uppercases names), and set only the
keys we own. Anything else stays untouched, including keys that do not exist
yet today. A system that is not present is appended. A new file gets
`servers:`, the spelling the other tools use.

- Before the first write, a copy is left next to the file as `<file>.bak`.
- The same invocation twice changes nothing the second time. This is a test,
  not an aspiration.
- `--print` renders without writing, for agents that want to show the user
  what will happen, and as an exit when a file is too exotic to touch.

Error messages name the flag and give an example, because a small model
correcting `--username` to `--user` needs to be told which one it is:

```
error: missing --user (TSO user id, e.g. IBMUSER)
error: --host needs a port, e.g. https://zos.example.com:443
```

New package `internal/setup`: `fields.go` (catalog), `prompt.go` (wizard),
`flags.go`, `document.go` (node editing), `env.go`. `internal/config` is not
modified beyond section 4; `--check` calls its loader.

## 4. Per-system passwords by convention

Today a system-specific password only works when the YAML names the variable:

```yaml
auth:
  password: "${ZOSMF_MAINT_PASSWORD}"
```

Without it the server falls back to one global `ZOSMF_PASSWORD` for every
system. Two LPARs with different passwords therefore force a construct into
the shared file that only this server reads, and couple two strings that must
match exactly — a mismatch surfaces as a startup error or an unexplained 401.

`normalize` gains one step: when a system has no password, try
`ZOSMF_<SYSTEM>_PASSWORD` before falling back to `ZOSMF_PASSWORD`. The
variable name derives from the system name, uppercased, with every character
outside `A-Z0-9` replaced by `_` — `PROD-A` becomes `ZOSMF_PROD_A_PASSWORD`.

This is additive. An explicit `${...}` in the file still wins, and
`ZOSMF_PASSWORD` remains the last resort, so existing installations are
unaffected.

At startup the server logs, per system, where the password came from: the
file, the system-specific variable, the global variable, or nowhere. The
value is never logged. Without this line, diagnosing the wrong password means
guessing which of three sources was used.

## 5. The agent skill

`agents/setup.md` holds the procedure once. Each supported agent gets a thin
wrapper in its own format, pointing at that file. The wrappers live in the
repository under `agents/`; nothing is installed into anyone's agent
configuration without being asked:

```bash
make install-skill                           # Claude, project scope → .claude/skills/
make install-skill AGENT=claude SCOPE=user   # → ~/.claude/skills/
make install-skill AGENT=opencode
make install-skill AGENT=pi
```

The target copies the wrapper together with `setup.md`, so the relative
reference survives. `.gitignore` keeps ignoring `.claude`.

The procedure stays short on purpose. Field names, help texts and defaults
are **not** in the text — they come from `--questions` at run time. The skill
says how to find out what to ask, not what to ask. That is what keeps it
usable by a small local model, and what keeps it at a few hundred tokens
instead of several thousand.

| Step | Action | Check |
| --- | --- | --- |
| 0 | `zosmf-mcp --version` | missing: point at the release or `make install`, stop here |
| 1 | `init config --questions`, ask, then `init config` with flags | `init config --check` reports nothing |
| 2 | "another LPAR?" → repeat step 1 | as above |
| 3 | `init env --questions`, then `init env` | file exists, mode 600 |
| 4 | choose the transport | present in the configuration |
| 5 | register with the client | the agent lists the server as connected |
| 6 | container, optional | `curl` against the port answers 401 |
| 7 | closing probe: one real tool call | the answer carries data, not an error |

Three rules are stated as prohibitions, because they are the failure modes of
a model working through this:

- **Never write YAML by hand.** Always through `init`.
- **Never print a password or an API key**, not even in an intermediate step.
- **On failure, show the error verbatim** and retry the corrected command
  instead of inventing another route.

Step 5 is the only one that differs per agent: a short table with
`claude mcp add` for Claude Code, the node configuration for n8n, and the
equivalent for opencode and pi.

`agents/setup.md` is readable without any agent. Someone who uses none works
through the same commands by hand.

## 6. Testing

The catalog, the writer and the environment file are ordinary Go code and are
tested as such. Priority is on the writer, because that is where damage to
someone else's file would happen.

- **Round trip:** a file with comments, `home`, `defaultServer` and an
  unknown key goes in; after an upsert everything except our keys is
  byte-identical. Golden files.
- **Upsert:** new system appended; existing system updated, not duplicated;
  name matching ignores case.
- **Idempotence:** the same invocation twice leaves the file unchanged the
  second time.
- **Lists:** `--csi A --csi B` and `--csi A,B` produce the same result.
- **Catalog:** every field reachable by a flag appears in `--questions`, and
  every catalog entry has a flag. A test over the field list, so the two
  cannot drift.
- **No TTY:** without flags and without a terminal the command fails with the
  message naming `--questions`, rather than blocking.
- **Password convention:** `ZOSMF_MAINT_PASSWORD` is preferred over
  `ZOSMF_PASSWORD`; an explicit `${...}` beats both; a name with a hyphen
  maps to an underscore. These tests clear the ambient environment, as the
  existing config tests do.
- **`--check`:** a configuration the loader rejects is reported as an error;
  a valid one passes.

`agents/setup.md` cannot be unit-tested. It is verified by running it: once
against a fresh machine state with Claude Code, once with a local model
through opencode, and the resulting configuration compared against one
written by hand.

## 7. Documentation

- README: a short "Getting started" section at the top that points at
  `make install-skill` and the two commands, in front of the existing manual
  instructions. The manual path stays — it is what the wizard automates.
- `docs/container-setup.md`: step 6 of the skill references it; no change to
  its content.
- `CHANGELOG.md`: under Unreleased, then released as 0.7.0.

## 8. Open question to settle during implementation

Where exactly opencode and pi look for instructions, and in which format.
This is not guesswork material: the first implementation step is to find out
from their documentation and record the answer here. Acceptance criterion for
that step is a wrapper file for each that the respective agent actually
discovers — verified by running it, not by reading about it.
