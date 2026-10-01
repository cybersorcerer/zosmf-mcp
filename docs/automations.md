# Building automations

Recipes for using the server beyond single questions: monitoring, pipelines, several systems, hooks and scheduled runs.

[← README](../README.md)


The zosmf-mcp server can be used with Claude Code to build complex automations.

## Simple Automation: Job Monitor

Prompt:
```
Monitor all active jobs on SYSA every 5 minutes and notify me
when a job completes with a return code greater than 0.
Create a shell script monitor.sh that performs this check.
```

## Complex Automation: Patch Verification

Prompt:
```
Check whether the following SYSMODs are installed on SYSA: UA12345, UA12346, UA12347.
Create a table showing the status (installed / not installed),
installation date, and the corresponding FMID for each.
```

## Deployment Pipeline

Prompt:
```
Perform the following steps on SYSA:
1. Read the JCL template from SYS1.PROCLIB(DEPLOY)
2. Replace &VERSION with "V2R1" and &DATE with today's date
3. Submit the modified job
4. Wait for completion and show me the return code
5. If RC > 0, show me the JESMSGLG spool file
```

## System Health Check

Prompt:
```
Run a full health check on SYSA:
1. System time and date
2. All active address spaces
3. Mounted filesystems
4. OPERLOG from the last 5 minutes — check for error messages
Summarize the results as a report.
```

## Multiple Systems

When multiple systems are configured, they can be compared in a single prompt:

```
Compare the installed version of SYSMOD HBB77E0 on SYSA and SYSB.
Are both systems at the same maintenance level?
```

## Claude Code Hooks

With Claude Code hooks, actions can be triggered automatically. Example `.claude/settings.json`:

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

## Scheduled Tasks with Claude Code

Use the `/schedule` command in Claude Code to set up recurring tasks:

```
/schedule daily at 08:00 AM:
Check on SYSA whether all critical started tasks (TCPIP, VTAM, JES2) are active.
If any are missing, generate an alert.
```


