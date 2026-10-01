# zosmf-mcp guided setup

This file is a procedure. Do the steps in order. Each step says what to run,
what the output must show, and what to do if it does not. Do not skip a
step. Do not do anything this file does not say.

The user enters every setting and every password themselves, in the
zosmf-mcp setup program, in their own terminal. Your job is to tell them
what to run, check the result, and connect their AI tool.

A human without an agent can follow the same steps by hand.

## Rules — they apply to every step

1. Run only the commands written in this file. Never change a file with an
   editor, `echo`, `sed`, `cat >` or a heredoc. The one exception is in
   step 6, and it says so.
2. Never read the configuration file or the `.env` file — no `cat`, `less`,
   `head`, `grep`, no file-read tool.
3. Never ask the user for a setting, a password or an API key. The setup
   program asks for them. If the user types a password into the chat
   anyway: do not repeat it, do not use it, and tell them to change it
   later because it is now in the chat history.
4. Never run `zosmf-mcp init config` or `zosmf-mcp init env` yourself
   without `--show`, `--check` or `--questions`. The user runs them.
5. If a command fails: show its output to the user unchanged and stop at
   that step until it is solved.

## Placeholders

- `<CFG>` — the configuration file. Step 1 tells you its path.
- `<BIN>` — the full path of the program. Step 0 tells you.

Replace a placeholder with its value. Never type the angle brackets.

## Step 0 — is the program installed?

Run: `command -v zosmf-mcp`

- Output is a path, for example `/Users/youruser/.local/bin/zosmf-mcp`:
  that is `<BIN>`. Go to step 1.
- No output: tell the user "zosmf-mcp is not installed. Download it from
  https://github.com/cybersorcerer/zosmf-mcp/releases/latest and put it on
  your PATH." Stop here.

## Step 1 — which file?

Run: `zosmf-mcp init config --show`

The output is JSON. The value of `"file"` is `<CFG>`.

Tell the user: "I will use the configuration file <CFG>." If they want a
different file, ask for its full path and use that as `<CFG>`.

## Step 2 — the user configures the systems

Tell the user exactly this, with `<CFG>` replaced:

> Please open a terminal of your own — not this chat — and run:
>
> `zosmf-mcp init config --file '<CFG>'`
>
> It shows the systems already configured and a menu: `n` adds a system,
> a number changes one, `r` restores an earlier version, `d` ends. It
> asks every question, shows each system before writing it and keeps a
> backup of the old file. Tell me "done" when you have ended it with `d`.

Wait until the user says "done".

## Step 3 — is the file valid?

Run: `zosmf-mcp init config --check --file '<CFG>'`

- The first line is `<CFG> is valid`: go to step 4.
- Anything else: show the output to the user unchanged and tell them: "Run
  `zosmf-mcp init config --file '<CFG>'` again and change the system the
  message names." When they say "done", run this step again.

## Step 4 — passwords

Run: `zosmf-mcp init env --questions --file '<CFG>'`

- The output contains `"fields": []`: every password is set up. Go to
  step 5.
- Otherwise tell the user exactly this, with `<CFG>` replaced:

  > Please run this in your own terminal — not this chat:
  >
  > `zosmf-mcp init env --file '<CFG>'`
  >
  > It asks for each password and saves it in a file only you can read.
  > Tell me "done" when it has finished.

  Wait for "done", then run this step again.

## Step 5 — does the server find the passwords?

Run: `zosmf-mcp init config --check --file '<CFG>'`

Look at the lines that start with `system ` and `warning:`.

- A line starts with `warning: ignored`: tell the user to run the
  `chmod 600` command shown in that line, in their own terminal. When they
  say "done", run this step again.
- A line ends in `password from none`: go back to step 4.
- Every `system ` line ends in `env-file`, `system-env`, `global-env` or
  `file`: go to step 6.

## Step 6 — connect the AI tool

zosmf-mcp reads its passwords itself. The AI tool only needs to start it.
Ask the user which tool to connect, then do only that part.

**Claude Code** — run:
`claude mcp add --scope user zosmf-mcp -- '<BIN>' --config '<CFG>'`
Then run `claude mcp list`. It must show `zosmf-mcp` with `✔ Connected`.

**opencode** — opencode reads MCP servers from its configuration file.

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

- If the file exists, do not change it. Show the user this block with the
  placeholders replaced, and ask them to add it inside the top-level `{ }`
  of that file and restart opencode:

  ```json
  "mcp": {
    "zosmf-mcp": {
      "type": "local",
      "command": ["<BIN>", "--config", "<CFG>"]
    }
  }
  ```

Then run `opencode mcp list`. It must show `zosmf-mcp` as connected.

**pi** — pi has no built-in MCP client. Tell the user: "pi needs the
pi-mcp-adapter extension to use MCP servers; its documentation says how to
add one. The program to start is `<BIN>` with the arguments `--config` and
`<CFG>`." Nothing to run.

**n8n or a container** — tell the user to follow `docs/container-setup.md`
in the zosmf-mcp repository. A container has its own `.env` and API key,
described there. Nothing to run from this file.

## Step 7 — final test

Ask the user to give their AI tool this request: "Use zosmf-mcp to list my
jobs." (A new session of the tool may be needed before it sees zosmf-mcp.)

- The answer lists jobs, or says there are none: setup is complete.
- The answer is an error: show it to the user unchanged. An error about
  the host or the certificate: go back to step 2. An error about the
  password or `401`: go back to step 4.

## Undo a change

Tell the user: "Run `zosmf-mcp init config --file '<CFG>'` in your own
terminal and choose `r`. It lists the saved versions, newest first; pick
the one to restore." Never choose a version yourself. When they say
"done", go to step 3.
