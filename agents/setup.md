# zosmf-mcp guided setup

The canonical setup procedure. Claude Code, opencode, pi, or a human at a
terminal all follow the same steps. Each step has one command and one
check. A failed check means stop, fix the one thing it names, and run the
same command again — not move on, not try something else.

`zosmf-mcp` must be on `PATH`, or use its full path throughout. `<CFG>` is
whatever `zosmf-mcp init config` printed as the file it wrote — read it from
that command's own output, do not guess a path. `<...>` marks a value to
fill in and `[...]` marks something optional; neither is typed literally.

## Three rules

1. **Never write YAML by hand.** Every change to the configuration goes
   through `zosmf-mcp init config`. Not a text editor, not a heredoc.
2. **Never print a password or an API key.** Not the value you generated,
   not a value read back from `.env`, not in an error message you compose
   yourself — not even to explain what you just did.
3. **On failure, show the error verbatim and retry the corrected command.**
   Do not invent a different way to reach the same result.

## Procedure

0. `zosmf-mcp --version`
   Check: prints a version line. Missing: point at the release download or
   `make install`, and stop here — nothing after this step works without it.

1. `zosmf-mcp init config --questions`
   Read the JSON it prints: it names every field, its type, whether it
   takes one value or several, and an example. Ask the user for each one,
   in that order, nothing more, nothing invented.
   Then write it: `zosmf-mcp init config --name <NAME> --host <URL> --user
   <USERID> [--csi A,B] [--zones A,B]` — one flag per answer, the flag name
   is the field's `id` from step 1's JSON. Include every field the user
   answered, not only these four. A bad flag value fails this command
   itself, naming the flag — fix it and run the same command again.
   Check: `zosmf-mcp init config --check` prints "... is valid" and exits 0.
   A non-zero exit here names the setting it rejected, not a flag — match
   it to the field from step 1 and correct the value with the write
   command.

2. Another LPAR? Repeat step 1 with its values. Same check.

3. `zosmf-mcp init env --questions`, then `zosmf-mcp init env` — or
   `zosmf-mcp init env --generate-api-key` when the client in step 5 or 6
   needs `MCP_API_KEY` and nothing else has generated one yet.
   With no terminal to answer on, the command names the missing variable
   and says so instead of hanging — export it and run the same command
   again.
   Check: the file it wrote exists and is mode 600 (`stat -f %Lp .env` on
   macOS/BSD, `stat -c %a .env` on Linux, must print `600`).

4. Prove the password actually reaches the server before wiring up a
   client: `set -a; . ./.env; set +a; timeout 2 zosmf-mcp --config <CFG>
   --transport stdio 2>&1 | tee /tmp/zosmf-mcp-start.log`.
   Check: the log has one `password from system-env` or `password from
   file` line per system — never `password from none`. `from none`: the
   variable step 3 wrote is not reaching the process — confirm it was
   exported (`set -a`) before this command ran; for a stdio client, pass
   the variable with the client's own env mechanism instead (see step 6).

5. Transport: `init config` already added `stdio: true` to `<CFG>` if the
   file had no transport block yet, so a client that runs the binary itself
   (Claude Code, Claude Desktop, VS Code, Cursor, opencode) needs nothing
   extra in step 6. `streamable-http` (a container, n8n) is never written to
   the file — name it explicitly with `--transport streamable-http` on that
   invocation, in step 7.
   Check: `grep -q 'stdio: true' <CFG>` exits 0, or the command you run in
   step 7 names `--transport streamable-http` explicitly. Neither: `<CFG>`
   already carries a different transport block from an earlier setup — add
   `--transport stdio` explicitly to the step 6 command instead of editing
   the file.

6. Register with the client — the command differs per agent, see the
   table below. A stdio client that does not inherit a sourced shell
   environment needs the `.env` variables passed through its own `env:`
   mechanism (the table below shows `--env` for Claude Code).
   Check: `claude mcp list` / `opencode mcp list` shows zosmf-mcp as
   connected, or the n8n node's own connection test succeeds — the table
   has the exact string for each client.

7. Container, optional — see `docs/container-setup.md`; it already passes
   `--transport streamable-http` for you.
   Check: `curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8081/mcp`
   prints `401` (no Authorization header sent yet — this confirms the
   server answers and the API key is enforced, not that you are in).

8. Closing probe: have the client make one real zosmf-mcp tool call, for
   example listing jobs on the system just configured.
   Check: the answer carries data — a job list, a dataset list, whatever
   the tool returns — not an error. An error here is a configuration
   problem, not a setup-procedure problem: read what it names and go back
   to step 1 or step 3, never around this file.

## Step 6 — registering with a client

| Client | Command / configuration | Check |
| --- | --- | --- |
| Claude Code | `claude mcp add zosmf-mcp --env ZOSMF_<SYSTEM>_PASSWORD=<value> -- <path-to-zosmf-mcp> --config <CFG>` (repeat `--env` per variable from step 3; omit it if the shell that starts Claude Code already sourced `.env`) | `claude mcp list` shows `zosmf-mcp` as `✔ Connected` |
| opencode | `opencode mcp add zosmf-mcp -- <path-to-zosmf-mcp> --config <CFG>` | `opencode mcp list` shows `zosmf-mcp` as `connected` |
| pi | pi has no built-in MCP client by design. If the `pi-mcp-adapter` extension is installed, it reads the same `.mcp.json` the Claude Code row writes — otherwise use pi to run this procedure for a client that does have one. | not applicable, or `/mcp` (adapter installed) shows zosmf-mcp connected |
| n8n | Run the server with `--transport streamable-http` (the container already does). In n8n, add an **MCP Client Tool** node: transport `Streamable HTTP`, URL `http://<host>:8081/mcp`. | the node's connection test succeeds |

Someone using no agent at all follows the same commands by hand — that is
what this file is for.
