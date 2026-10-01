# zosmf-mcp guided setup

This file is a procedure. Do the steps in order. Each step says what to run,
what the output must show, and what to do if it does not. Do not skip a
step. Do not do anything this file does not say.

A human without an agent can follow the same steps by hand.

## Rules — read these first, they apply to every step

1. Run only the commands written in this file. Never change a file with an
   editor, `echo`, `sed`, `cat >` or a heredoc. The one exception is in
   step 9, and it says so.
2. Never read the configuration file or the `.env` file — no `cat`, `less`,
   `head`, `grep`, no file-read tool. `zosmf-mcp init config --show` tells
   you everything you need.
3. Never ask the user for a password or an API key, and never write one
   into a command, a file or a message. If the user types a password into
   the chat anyway: do not repeat it, do not use it, and tell them to
   change it later because it is now in the chat history.
4. Use the user's answers exactly as given. Do not add, complete, shorten
   or correct a value. If an answer looks wrong or incomplete, ask again —
   do not fix it yourself.
5. In every command, put every value in single quotes: `--user 'IBMUSER'`.
   Write lists as one value, entries separated by commas, no spaces:
   `--zones 'MVST100,MVSD100'`.
6. If a command fails: show its error message to the user unchanged. Change
   only the value the message names (ask the user for it), then run the
   same command again. Never try a different command instead.
7. Ask one question per message. Wait for the answer before the next one.

## Placeholders

- `<CFG>` — the configuration file. Step 1 tells you its path.
- `<BIN>` — the full path of the program. Step 0 tells you.
- `<NAME>` — the system name the user chose in step 3.

Replace a placeholder with its value. Never type the angle brackets.

## Step 0 — is the program installed?

Run: `command -v zosmf-mcp`

- Output is a path, for example `/Users/youruser/.local/bin/zosmf-mcp`: that is
  `<BIN>`. Go to step 1.
- No output: tell the user "zosmf-mcp is not installed. Download it from
  https://github.com/cybersorcerer/zosmf-mcp/releases/latest and put it on
  your PATH." Stop here.

## Step 1 — what is configured already?

Run: `zosmf-mcp init config --show`

The output is JSON. `"file"` is `<CFG>`. `"systems"` lists the systems
already in the file, each with its `"name"` and its `"values"`.

Tell the user: "I will use the configuration file <CFG>." If they want a
different file, ask for its full path, use that as `<CFG>`, and run
`zosmf-mcp init config --show --file '<CFG>'` instead.

- `"systems": []`: nothing is configured yet. Go to step 2.
- Otherwise tell the user the names and the `"host"` of each system, then
  go to step 2.

## Step 2 — what does the user want?

If step 1 listed no systems, the answer is "add a system": go to step 3.

Otherwise ask exactly:

> Do you want to add a new system, change one of these systems, or only
> set up passwords and the connection to your AI tool?

- Add a new system: go to step 3.
- Change a system: ask which one. That name is `<NAME>`. Go to step 4.
  When you change a system, the values from step 1 are the current ones:
  for each question in step 4, tell the user the current value and ask
  whether to keep it.
- Only passwords and connection: go to step 7.

## Step 3 — the system name

Ask exactly:

> What name do you want to use for this z/OS system? Use letters and
> digits, for example MAIN.

The answer is `<NAME>`. If step 1 already lists a system with this name
(upper or lower case does not matter), tell the user and ask whether they
want to change that system instead. If yes, continue as "change a system".

## Step 4 — ask the questions

Ask these questions one at a time, in this order. Write down each answer
and the flag it belongs to. A question marked *(skip if …)* is not asked in
that case.

1. Ask: "What is the full z/OSMF URL? Start it with https:// — for example
   https://zos.example.com:443"
   Flag: `--host`. The answer must start with `https://` or `http://`. If
   it does not, ask: "Please give the URL with https:// at the start." Do
   not add `https://` yourself.
2. Ask: "Which z/OS user ID should zosmf-mcp log in with? For example
   IBMUSER"
   Flag: `--user`.
3. Ask: "Does this z/OSMF use a certificate that your computer does not
   trust, for example a self-signed one? Answer yes or no."
   - no: nothing to write.
   - yes: ask "Do you have the CA certificate as a PEM file on this
     computer? If yes, give me its full path. If no, say no."
     - a path: flag `--ca-file`.
     - no: flag `--reject-unauthorized` with the value `false`. Tell the
       user: "Certificate checks are now off for this system. Use this only
       for a test system."
4. Ask: "Do you want default SMP/E settings, so that SMP/E queries work
   without naming a CSI and zone each time? Answer yes or no."
   - yes: ask "Which CSI data sets? Give one per line. The first one is
     the default." Flag: `--csi`.
     Then ask "Which zones should a query use when it names none? One per
     line, for example MVST100." Flag: `--zones`.
5. Ask: "Do you want to use TSO commands through zosmf-mcp? Answer yes or
   no."
   - yes: ask "What is your TSO account number?" Flag: `--tso-account`.
     Then ask "What is your TSO logon procedure? For example IZUFPROC"
     Flag: `--tso-proc`.
6. Ask: "Do you want ZOAU tools? They run commands on z/OS over SSH. Answer
   yes or no."
   - yes: ask these four questions, one at a time:
     1. "What is the SSH host name or IP address of the z/OS system,
        without https://? For example zos.example.com" Flag: `--ssh-host`.
     2. "Which z/OS user should the SSH login use?" Flag: `--ssh-user`.
     3. "What is the full path of the SSH private key file on the computer
        where zosmf-mcp runs? Inside a container, give the path inside the
        container." Flag: `--ssh-key-file`.
     4. "In which z/OS directory is ZOAU installed? For example
        /usr/lpp/IBM/zoautil" Flag: `--zoau-path`.
     If the SSH port is not 22, the user will say so; then add the flag
     `--ssh-port`. Do not ask about it otherwise.

## Step 5 — preview, then write

Build the command. Start with:

`zosmf-mcp init config --file '<CFG>' --name '<NAME>' --host '<URL>' --user '<USER>'`

Add one flag with its value for every other answer from step 4. Add
nothing else. Example with SMP/E and TSO answers:

`zosmf-mcp init config --file '/Users/youruser/.config/smpe_ls/.smpe-zosmf.yaml' --name 'MAIN' --host 'https://zos.example.com:443' --user 'IBMUSER' --csi 'ZOS31.GLOBAL.CSI' --zones 'MVST100,MVSD100' --tso-account 'ACCT#' --tso-proc 'IZUFPROC'`

1. Run the command with ` --print` added at the end. It writes nothing; it
   prints the system as it would be written.
   - It fails: follow rule 6.
   - It succeeds: show the output to the user and ask "Shall I write this?
     Answer yes or no."
   - no: ask what to change, change only that, and run step 5 again.
2. yes: run the same command without ` --print`.
   It must print `wrote <CFG>` and `added system <NAME>` or
   `updated system <NAME>`. It also names the backup of the old file.
3. Run: `zosmf-mcp init config --check --file '<CFG>'`
   It must print `<CFG> is valid`. If it prints an error instead, follow
   rule 6.

Ask: "Do you want to set up another z/OS system?" If yes, go back to
step 3.

## Step 6 — container paths

If the user will run zosmf-mcp in a container, tell them: "File paths you
gave me (key file, CA file) must be paths inside the container." Nothing
to run.

## Step 7 — passwords

Run: `zosmf-mcp init env --questions --file '<CFG>'`

- `"fields": []`: every password is already set up. Go to step 8.
- Otherwise each entry in `"fields"` has an `"id"`, for example
  `ZOSMF_MAIN_PASSWORD`. Do not ask for these values yourself. Tell the
  user exactly:

  > Please open a terminal of your own — not this chat — and run:
  > `zosmf-mcp init env --file '<CFG>'`
  > It asks for each password and saves it in a file only you can read.
  > Tell me "done" when it has finished.

  Wait for "done". Then run the `--questions` command above again. It must
  now show `"fields": []`. If it does not, ask the user to run the command
  again.

## Step 8 — does the server find the passwords?

Run: `zosmf-mcp init config --check --file '<CFG>'`

The output has one line per system: `system <NAME>: password from ...`.

- Every line ends in `env-file`, `system-env`, `global-env` or `file`:
  good. Go to step 9.
- A line ends in `none`: that system has no password. Go back to step 7.
- A line starts with `warning: ignored`: the password file can be read by
  other users. Tell the user to run the `chmod 600` command the warning
  shows, in their own terminal. Then run this step again.

## Step 9 — connect the AI tool

zosmf-mcp reads its passwords itself. The AI tool only needs to start it.
Ask which tool the user wants to connect, then do only that row.

**Claude Code** — run:
`claude mcp add --scope user zosmf-mcp -- '<BIN>' --config '<CFG>'`
Then run `claude mcp list`. It must show `zosmf-mcp` with `✔ Connected`.

**opencode** — opencode reads MCP servers from its configuration file. The
block to add is:

```json
"mcp": {
  "zosmf-mcp": {
    "type": "local",
    "command": ["<BIN>", "--config", "<CFG>"]
  }
}
```

- If the file `~/.config/opencode/opencode.json` does not exist, you may
  create it (this is the exception to rule 1) with exactly this content,
  placeholders replaced:

  ```json
  {
    "$schema": "https://opencode.ai/config.json",
    "mcp": {
      "zosmf-mcp": {
        "type": "local",
        "command": ["<BIN>", "--config", "<CFG>"]
      }
    }
  }
  ```

- If the file exists, do not change it. Show the user the block above with
  the placeholders replaced and ask them to add it to the file themselves.
  Tell them to restart opencode afterwards.

Then run `opencode mcp list`. It must show `zosmf-mcp` as connected.

**pi** — pi has no built-in MCP client. Tell the user: "pi needs the
pi-mcp-adapter extension to use MCP servers; its documentation says how to
add one. The command to start is: `<BIN>` with the arguments `--config`
and `<CFG>`." Nothing to run.

**n8n or a container** — tell the user to follow `docs/container-setup.md`
in the zosmf-mcp repository. A container has its own `.env` and API key,
described there. Nothing to run from this file.

## Step 10 — final test

Ask the user to give their AI tool this request: "Use zosmf-mcp to list my
jobs on <NAME>."

- The answer lists jobs (or says there are none): setup is complete.
- The answer is an error: show it to the user unchanged. An error that
  names the host or the certificate: go back to step 2 and change the
  system. An error about the password or `401`: go back to step 7.

## Undo a change

Every write keeps a copy of the file it replaced.

1. Run: `zosmf-mcp init restore --list --file '<CFG>'`
   Show the user the list: `target`, `timestamp`, `file`. Ask which one to
   restore. Never choose yourself.
2. Run: `zosmf-mcp init restore --file '<CFG>' --backup '<TIMESTAMP>'`
   If it says both match, add `--target 'config'` or `--target 'env'` —
   ask the user which one.
3. Run: `zosmf-mcp init config --check --file '<CFG>'`. It must print
   `<CFG> is valid`.

The restore output names where the replaced version went, so a wrong
choice can be undone the same way.
