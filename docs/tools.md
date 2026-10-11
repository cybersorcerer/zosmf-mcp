# Tools and resources

[← README](../README.md)

Every tool the server registers, grouped by area, with example prompts: 34 tools in 9 areas, plus 11 ZOAU tools when `ssh_systems` is configured. The tools that can destroy data are also listed in the README under [Before You Start](../README.md#️-before-you-start).

## Tool annotations

Every tool carries MCP annotations (`readOnlyHint`, `destructiveHint`,
`idempotentHint`, `openWorldHint`). Clients use them to decide what may run
unattended. Read-only tools such as `zosmf_list_datasets` are safe to
auto-approve; tools marked destructive — `zosmf_delete_dataset`,
`zosmf_write_dataset`, `zosmf_purge_job`, `zosmf_cancel_job`,
`zosmf_unmount_filesystem`, `zosmf_delete_zfs`, `zosmf_delete_uss_file`,
`zosmf_write_uss_file`, `zosmf_issue_command`, `zosmf_tso_command` and
`zoau_exec` — change or destroy state and should stay behind a confirmation.

The `system` parameter of each tool is enumerated from your configuration, so
a client sees the systems you actually have instead of guessing a name.

## Jobs (10 tools)
| Tool                      | Description                                  |
|---------------------------|----------------------------------------------|
| `zosmf_list_jobs`         | List jobs with optional filters              |
| `zosmf_get_job`           | Get details of a specific job                |
| `zosmf_get_job_output`    | List spool files (DD names) of a job         |
| `zosmf_get_spool_content` | Read the content of a spool file             |
| `zosmf_get_all_spools`    | Read all spool file contents of a job in a single call |
| `zosmf_submit_job`        | Submit a job from inline JCL or a dataset    |
| `zosmf_cancel_job`        | Cancel a running job                         |
| `zosmf_purge_job`         | Delete job output from the spool             |
| `zosmf_hold_job`          | Place a job on hold                          |
| `zosmf_release_job`       | Release a held job                           |

**Example prompts**

**Show all active jobs**
```
Show me all currently active jobs on SYSA
```

**Jobs for a specific owner**
```
Which jobs are currently running under user IBMUSER on SYSA?
```

**All OUTPUT jobs as a table**
```
List all jobs in OUTPUT status on SYSA and show the result as a table
```

**Read job output**
```
Show me the content of the JESMSGLG spool file for job MYJOB JOB12345 on SYSA
```

**Submit a job**
```
Submit the following job on SYSA:
//TESTJOB JOB (ACCT),'Test',CLASS=A,MSGCLASS=X
//STEP1   EXEC PGM=IEFBR14
```

**Submit a job from a dataset**
```
Submit the job from dataset USER.JCL(MYJOB) on SYSA
```

**Cancel a job**
```
Cancel job LONGJOB JOB00042 on SYSA
```

**Delete job output**
```
Delete the spool output of job TESTJOB JOB12345 on SYSA
```


## SMP/E (1 tool)
| Tool                   | Description                                      |
|------------------------|--------------------------------------------------|
| `zosmf_smpe_query_csi` | Query the SMP/E CSI (SYSMODs, DDDEFs, products) |

**Example prompts**

**Check whether a SYSMOD is installed**
```
Is SYSMOD UA12345 installed on SYSA in target zone TGTZONE?
```

**Show all SYSMODs for a FMID**
```
Show me all installed SYSMODs for FMID HBB77E0 on SYSA
```

**Look up a DDDEF dataset name**
```
What is the dataset name of DDDEF entry SYSLIB in the target zone on SYSA?
```

**Show all target zones**
```
Show me all configured target zones in the CSI on SYSA
```

**Query product information**
```
Show me all installed products on SYSA
```

**Search for error HOLDDATAs**
```
Are there any open HOLDDATAs of type ERROR on SYSA in the target zone?
```


## Datasets (8 tools)

| Tool                         | Description                                              |
|------------------------------|----------------------------------------------------------|
| `zosmf_list_datasets`        | Search datasets by pattern; optional `volser` and `start` parameters |
| `zosmf_list_members`         | List members of a PDS or PDSE                            |
| `zosmf_read_dataset`         | Read a sequential dataset or PDS member                  |
| `zosmf_write_dataset`        | Write to a sequential dataset or PDS member; optional `release_enq` to release ENQ after write |
| `zosmf_create_dataset`       | Allocate a new dataset                                   |
| `zosmf_delete_dataset`       | Delete a dataset or PDS member                           |
| `zosmf_acquire_dataset_enq`  | Acquire an exclusive or shared ENQ lock on a dataset or member |
| `zosmf_release_dataset_enq`  | Release a previously acquired ENQ lock                   |

**Example prompts**

**Search datasets by pattern**
```
Find all datasets starting with SYS1.PARMLIB on SYSA
```

**List PDS members**
```
List all members of SYS1.PARMLIB on SYSA
```

**Read dataset content**
```
Show me the content of SYS1.PARMLIB(IEASYS00) on SYSA
```

**Write a PDS member**
```
Write the following content to USER.JCL(NEWJOB) on SYSA: [JCL...]
```

**Allocate a new dataset**
```
Allocate a new PDS dataset USER.TEST.PDS on SYSA with
DSORG=PO, RECFM=FB, LRECL=80, 5 tracks primary, 1 track secondary, 10 directory blocks
```

**Delete a dataset**
```
Delete dataset USER.TEMP.DATA on SYSA
```


## Filesystems (5 tools)
| Tool                       | Description                              |
|----------------------------|------------------------------------------|
| `zosmf_list_filesystems`   | List mounted filesystems                 |
| `zosmf_mount_filesystem`   | Mount a filesystem                       |
| `zosmf_unmount_filesystem` | Unmount a filesystem                     |
| `zosmf_create_zfs`         | Create a new zFS dataset                 |
| `zosmf_delete_zfs`         | Delete a zFS dataset                     |

**Example prompts**

**Show all mounted filesystems**
```
Show me all filesystems mounted on SYSA
```

**Filter filesystems by mount path**
```
Which filesystems are mounted under /usr on SYSA?
```

**Mount a filesystem**
```
Mount zFS dataset USER.MY.ZFS on SYSA at path /u/myuser/work in read-write mode
```

**Create a new zFS**
```
Create a new zFS dataset USER.NEW.ZFS on SYSA with 50 primary cylinders
```


## Authentication (1 tool)

| Tool                 | Description                                                |
|----------------------|------------------------------------------------------------|
| `zosmf_set_password` | Set the password for a system at runtime (in-memory only)  |

The password lasts until the server restarts. It helps when a call failed with `NO_CREDENTIALS` or `AUTH_FAILED` and the `.env` cannot be fixed right away — but the password then travels through the conversation, and the client may keep it in its transcript. The [`.env`](configuration.md#passwords) is the better place.

## Operator Console (2 tools)
| Tool                     | Description                              |
|--------------------------|------------------------------------------|
| `zosmf_issue_command`    | Issue an operator command                |
| `zosmf_get_hardcopy_log` | Read OPERLOG or SYSLOG                   |

**Example prompts**

**Query system time**
```
What time is it on SYSA right now?
```
*(issues operator command `D T`)*

**Show all active address spaces**
```
Show me all active address spaces on SYSA
```
*(issues `D A,L`)*

**Check CPU utilization**
```
Show me the current CPU utilization on SYSA
```
*(issues `D M=CPU`)*

**Start a started task**
```
Start the started task MYAPP on SYSA
```
*(issues `S MYAPP`)*

**Retrieve the last 30 minutes of OPERLOG**
```
Show me the operator log messages from the last 30 minutes on SYSA
```

**Search SYSLOG for errors**
```
Are there any error messages in the SYSLOG on SYSA from the last 10 minutes?
```


## USS Files (5 tools)
| Tool                     | Description                              |
|--------------------------|------------------------------------------|
| `zosmf_list_uss_files`   | List a USS directory                                                        |
| `zosmf_read_uss_file`    | Read a USS file; optional `tag` for encoding selection (`t IBM-1047`, `b`) |
| `zosmf_write_uss_file`   | Write to a USS file; optional `tag` for encoding selection                 |
| `zosmf_create_uss_file`  | Create a USS file or directory                                              |
| `zosmf_delete_uss_file`  | Delete a USS file or directory                                              |

**Example prompts**

**List a directory**
```
List the contents of directory /u/ibmuser on SYSA
```

**Read a file**
```
Show me the content of /u/ibmuser/.profile on SYSA
```

**Write a file**
```
Write the following content to /u/ibmuser/test.sh on SYSA: [script...]
```

**Create a new file**
```
Create the file /u/ibmuser/config.txt on SYSA
```

**Create a new directory**
```
Create directory /u/ibmuser/projects on SYSA with permissions rwxr-xr-x
```

**Delete a file**
```
Delete the file /u/ibmuser/old.log on SYSA
```


## TSO (1 tool)
| Tool                  | Description                                      |
|-----------------------|--------------------------------------------------|
| `zosmf_tso_command`   | Execute a TSO/E command and return the output    |

**Example prompts**

**Query the dataset catalog**
```
Run the TSO command LISTCAT LVL(USER) on SYSA
```

**Show dataset details**
```
Use TSO on SYSA to show details for dataset USER.JCL
```
*(issues `LISTDS USER.JCL`)*

**Check RACF permissions**
```
Use TSO on SYSA to check RACF permissions for dataset USER.SECURE.DATA
```
*(issues `LISTDSD DA(USER.SECURE.DATA) ALL`)*


## Parmlib (1 tool)

| Tool                     | Description                                    |
|--------------------------|------------------------------------------------|
| `zosmf_parmlib_validate` | Validate z/OS parmlib member syntax via z/OSMF |

**Example prompts**

**Validate all active BPXPRM members**
```
Validate the syntax of all active BPXPRM parmlib members on SYSA
```

**Validate a specific member**
```
Validate the syntax of SYS1.PARMLIB(IEASYS00) on SYSA
```

**Validate all parmlib types via LOADxx**
```
Validate all parmlib types deeply using LOADxx member LOAD00 from SYS1.PARMLIB on SYSA
```

**Validate inline content**
```
Validate the following BPXPRM parmlib content on SYSA: [parmlib text...]
```


## ZOAU (11 tools) — requires `ssh_systems` configuration

The ZOAU tools take an `ssh_system` parameter. It names an entry of `ssh_systems:` — a separate namespace from the `system` of the `zosmf_*` tools; see [ZOAU over SSH](configuration.md#zoau-over-ssh).

| Tool                      | Description                                          |
|---------------------------|------------------------------------------------------|
| `zoau_pll`          | Display the z/OS linklist (LLA) concatenation        |
| `zoau_zinfo`        | Display z/OS system information                      |
| `zoau_llwhence`     | Locate which linklist dataset a member comes from    |
| `zoau_parmgrep`     | Search parmlib members for a pattern                 |
| `zoau_parmwhence`   | Locate which parmlib dataset a member comes from     |
| `zoau_pproc`        | Display the z/OS proclib concatenation               |
| `zoau_procgrep`     | Search proclib members for a pattern                 |
| `zoau_procwhence`   | Locate which proclib dataset a member comes from     |
| `zoau_version`      | Display the installed ZOAU version                   |
| `zoau_vtocls`       | List the VTOC contents of a z/OS volume              |
| `zoau_exec`         | Execute a USS command or script via SSH; returns stdout, stderr and exit code as JSON         |

**Example prompts**

**Show the installed ZOAU version**
```
Which ZOAU version is installed on SYSA?
```

**Find where a parmlib member comes from**
```
Which parmlib dataset does member IEASYS00 come from on SYSA?
```

**Search the proclib**
```
Search the proclib on SYSA for procedures that reference SYS1.LINKLIB
```

**Show the linklist**
```
Show me the linklist concatenation on SYSA
```


## Resources

Besides tools, the server exposes z/OS objects as MCP resources. A client can
attach them as context directly, and tools return links to them instead of
dumping large content into the conversation.

| URI | What it addresses |
| --- | --- |
| `zosmf:///systems` | The configured z/OSMF endpoints |
| `zosmf://{system}/dataset/{dsn}` | A dataset or PDS member |
| `zosmf://{system}/uss/{path}` | A z/OS UNIX file |
| `zosmf://{system}/job/{jobname}/{jobid}/spool/{id}` | One spool file of a job |

Only `zosmf:///systems` appears in `resources/list` — the others are templates,
since a system's datasets cannot be enumerated.

Read results above `resources.inline_max_bytes` (32 KB by default) come back as
a preview plus a `resource_link`. `zosmf_get_all_spools` fills its budget with
the small JES files first and links the rest, so a job with a large SYSPRINT
still returns the messages that matter for diagnosis.

Resource access is bounded by SAF/RACF only, exactly like the read tools. See
[Before You Start](../README.md#️-before-you-start).


