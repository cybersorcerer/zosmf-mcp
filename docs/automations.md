# Building automations

[← README](../README.md)

Recipes for using the server beyond single questions: tasks with several steps, several systems, hooks and scheduled runs. Each recipe is a prompt for your AI client; the client picks the tools. Where a recipe changes something on z/OS, a well-behaved client asks before that call — see [Before You Start](../README.md#️-before-you-start).

## Patch verification

Reads the CSI only.

```
Check whether the following SYSMODs are installed on SYSA: UA12345, UA12346, UA12347.
Create a table showing the status (installed / not installed),
installation date, and the corresponding FMID for each.
```

## Failed jobs

Reads job lists and spool files only.

```
List the jobs on SYSA that ended with a return code greater than 0,
and show me the JESMSGLG of each.
```

## System health check

Uses display commands only (`D T`, `D A,L`), but through `zosmf_issue_command`, which can run any operator command — so the client asks before each call.

```
Run a full health check on SYSA:
1. System time and date
2. All active address spaces
3. Mounted filesystems
4. OPERLOG from the last 5 minutes — check for error messages
Summarize the results as a report.
```

## Deployment pipeline

Submits a job.

```
Perform the following steps on SYSA:
1. Read the JCL template from SYS1.PROCLIB(DEPLOY)
2. Replace &VERSION with "V2R1" and &DATE with today's date
3. Submit the modified job
4. Wait for completion and show me the return code
5. If RC > 0, show me the JESMSGLG spool file
```

## Several systems

With more than one system configured, one prompt can compare them:

```
Compare which PTFs for FMID HBB77E0 are installed on SYSA and SYSB.
Are both systems at the same maintenance level?
```

## Claude Code hooks

Claude Code hooks run a command when a tool has been used. Claude Code names MCP tools `mcp__<server>__<tool>`, so the matcher has to use that full name — here for a server registered as `zosmf-mcp`. Example `.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "mcp__zosmf-mcp__zosmf_submit_job",
        "hooks": [
          {
            "type": "command",
            "command": "echo \"Job submitted at $(date)\" >> job.log"
          }
        ]
      }
    ]
  }
}
```

## Scheduled runs

A scheduled run needs zosmf-mcp on a machine that can reach z/OS, so it has to run on your side:

- **Claude Desktop scheduled tasks** — under **Routines**, create a **Local** routine. It runs on your machine with the MCP servers from your configuration, while the app is open and the computer awake.
- **cron or launchd with `claude -p`**, Claude Code's non-interactive mode. Allow only the tools the run needs, and never the ones that change or destroy data:

  ```bash
  claude -p "Check on SYSA whether the jobs TCPIP, VTAM and JES2 are active. If one is not, start your answer with ALERT and name it." \
    --allowedTools "mcp__zosmf-mcp__zosmf_list_jobs"
  ```

- **`/loop`** repeats a prompt inside a running Claude Code session, for as long as the session stays open.

Cloud routines — `/schedule` in Claude Code, or **Cloud** under Routines in Claude Desktop — run on Anthropic's infrastructure. They cannot reach a zosmf-mcp on your machine or network, and opening one to the internet for them would expose your z/OS credentials.
