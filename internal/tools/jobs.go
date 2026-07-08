package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

// --- List Jobs ---

type listJobsTool struct {
	clients *zosmf.ClientRegistry
}

type listJobsArgs struct {
	System   string `json:"system"`
	Owner    string `json:"owner,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
	JobID    string `json:"jobid,omitempty"`
	Status   string `json:"status,omitempty"`
	ExecData bool   `json:"exec_data,omitempty"`
}

func NewListJobsTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &listJobsTool{clients: clients}
}

func (t *listJobsTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_list_jobs",
		Description: "List z/OS jobs on a system. Filter by owner, job name prefix, job ID, and/or status (ACTIVE, OUTPUT, INPUT). " +
			"To list ALL jobs (not just your own), set owner and prefix to '*'. " +
			"Set exec_data=true to include execution timestamps (exec-started, exec-ended, exec-submitted, exec-system, exec-member).",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{
					"type":        "string",
					"description": "Name of the z/OS system to query (as configured)",
				},
				"owner": map[string]any{
					"type":        "string",
					"description": "Job owner (TSO user ID). Use '*' for all owners. Supports wildcards (e.g. 'SYS*'). Defaults to the authenticated user if omitted.",
				},
				"prefix": map[string]any{
					"type":        "string",
					"description": "Job name prefix filter. Use '*' for all job names. Supports wildcards (e.g. 'IEF*'). Defaults to the authenticated user's prefix if omitted.",
				},
				"jobid": map[string]any{
					"type":        "string",
					"description": "Job ID filter (e.g. JOB12345). Mutually exclusive with user-correlator. Folded to uppercase.",
				},
				"status": map[string]any{
					"type":        "string",
					"description": "Job status filter: ACTIVE, OUTPUT, or INPUT. If omitted, all statuses are returned.",
					"enum":        []string{"ACTIVE", "OUTPUT", "INPUT"},
				},
				"exec_data": map[string]any{
					"type":        "boolean",
					"description": "If true, include exec-started, exec-ended, exec-member, exec-system fields in the response (adds exec-data=Y query parameter).",
				},
			},
			"required": []string{"system"},
		}),
	}
}

func (t *listJobsTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args listJobsArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	jobs, err := client.ListJobs(zosmf.ListJobsParams{
		Owner:    args.Owner,
		Prefix:   args.Prefix,
		JobID:    args.JobID,
		Status:   args.Status,
		ExecData: args.ExecData,
	})
	if err != nil {
		return mcp.ErrorResult("failed to list jobs: " + err.Error()), nil
	}

	data, _ := json.MarshalIndent(jobs, "", "  ")
	return mcp.TextResult(string(data)), nil
}

// --- Get Job ---

type getJobTool struct {
	clients *zosmf.ClientRegistry
}

type getJobArgs struct {
	System  string `json:"system"`
	JobName string `json:"job_name"`
	JobID   string `json:"job_id"`
}

func NewGetJobTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &getJobTool{clients: clients}
}

func (t *getJobTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_get_job",
		Description: "Get details of a specific z/OS job including status, return code, execution data (exec-system, exec-member, exec-submitted, exec-started, exec-ended) and step data (step name, number, completion code) for all completed steps.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":   map[string]any{"type": "string", "description": "Name of the z/OS system"},
				"job_name": map[string]any{"type": "string", "description": "Job name (e.g. MYJOB1)"},
				"job_id":   map[string]any{"type": "string", "description": "Job ID (e.g. JOB12345)"},
			},
			"required": []string{"system", "job_name", "job_id"},
		}),
	}
}

func (t *getJobTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args getJobArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	job, err := client.GetJob(args.JobName, args.JobID)
	if err != nil {
		return mcp.ErrorResult("failed to get job: " + err.Error()), nil
	}

	data, _ := json.MarshalIndent(job, "", "  ")
	return mcp.TextResult(string(data)), nil
}

// --- Get Job Output (spool file list) ---

type getJobOutputTool struct {
	clients *zosmf.ClientRegistry
}

func NewGetJobOutputTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &getJobOutputTool{clients: clients}
}

func (t *getJobOutputTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_get_job_output",
		Description: "List spool files (DD names) of a z/OS job. Use zosmf_get_spool_content to read individual spool file contents.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":   map[string]any{"type": "string", "description": "Name of the z/OS system"},
				"job_name": map[string]any{"type": "string", "description": "Job name"},
				"job_id":   map[string]any{"type": "string", "description": "Job ID"},
			},
			"required": []string{"system", "job_name", "job_id"},
		}),
	}
}

func (t *getJobOutputTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args getJobArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	files, err := client.GetJobFiles(args.JobName, args.JobID)
	if err != nil {
		return mcp.ErrorResult("failed to get job files: " + err.Error()), nil
	}

	data, _ := json.MarshalIndent(files, "", "  ")
	return mcp.TextResult(string(data)), nil
}

// --- Get Spool Content ---

type getSpoolContentTool struct {
	clients *zosmf.ClientRegistry
}

type getSpoolContentArgs struct {
	System  string          `json:"system"`
	JobName string          `json:"job_name"`
	JobID   string          `json:"job_id"`
	FileID  json.RawMessage `json:"file_id"`
}

func NewGetSpoolContentTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &getSpoolContentTool{clients: clients}
}

func (t *getSpoolContentTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_get_spool_content",
		Description: "Read the content of a specific spool file (DD) from a z/OS job. Use zosmf_get_job_output first to get available file IDs.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":   map[string]any{"type": "string", "description": "Name of the z/OS system"},
				"job_name": map[string]any{"type": "string", "description": "Job name"},
				"job_id":   map[string]any{"type": "string", "description": "Job ID"},
				"file_id":  map[string]any{"type": "string", "description": "Spool file ID (from zosmf_get_job_output)"},
			},
			"required": []string{"system", "job_name", "job_id", "file_id"},
		}),
	}
}

func (t *getSpoolContentTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args getSpoolContentArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	var fileID int
	// file_id may arrive as integer or quoted string — handle both
	raw := string(args.FileID)
	raw = strings.Trim(raw, `"`)
	if _, err := fmt.Sscanf(raw, "%d", &fileID); err != nil {
		return mcp.ErrorResult("invalid file_id: " + err.Error()), nil
	}
	content, err := client.GetJobFileContent(args.JobName, args.JobID, fileID)
	if err != nil {
		return mcp.ErrorResult("failed to get spool content: " + err.Error()), nil
	}

	return mcp.TextResult(content), nil
}

// --- Get All Spools ---

type getAllSpoolsTool struct {
	clients *zosmf.ClientRegistry
}

func NewGetAllSpoolsTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &getAllSpoolsTool{clients: clients}
}

func (t *getAllSpoolsTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_get_all_spools",
		Description: "Read all spool file contents of a z/OS job in a single call. Returns all DD names and their content concatenated.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":   map[string]any{"type": "string", "description": "Name of the z/OS system"},
				"job_name": map[string]any{"type": "string", "description": "Job name"},
				"job_id":   map[string]any{"type": "string", "description": "Job ID"},
			},
			"required": []string{"system", "job_name", "job_id"},
		}),
	}
}

func (t *getAllSpoolsTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args getJobArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	files, err := client.GetJobFiles(args.JobName, args.JobID)
	if err != nil {
		return mcp.ErrorResult("failed to get job files: " + err.Error()), nil
	}

	var result strings.Builder
	for _, f := range files {
		content, err := client.GetJobFileContent(args.JobName, args.JobID, f.ID)
		if err != nil {
			result.WriteString(fmt.Sprintf("--- %s (step: %s) ---\n(error: %s)\n\n", f.DDName, f.StepName, err.Error()))
			continue
		}
		result.WriteString(fmt.Sprintf("--- %s (step: %s) ---\n%s\n\n", f.DDName, f.StepName, content))
	}

	return mcp.TextResult(result.String()), nil
}

// --- Submit Job ---

type submitJobTool struct {
	clients *zosmf.ClientRegistry
}

type submitJobArgs struct {
	System  string `json:"system"`
	JCL     string `json:"jcl,omitempty"`
	Dataset string `json:"dataset,omitempty"`
}

func NewSubmitJobTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &submitJobTool{clients: clients}
}

func (t *submitJobTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_submit_job",
		Description: "Submit a z/OS job. Provide either inline JCL or a dataset name containing the JCL.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":  map[string]any{"type": "string", "description": "Name of the z/OS system"},
				"jcl":     map[string]any{"type": "string", "description": "Inline JCL to submit"},
				"dataset": map[string]any{"type": "string", "description": "Dataset name containing JCL (e.g. USER.JCL(MYJOB))"},
			},
			"required": []string{"system"},
		}),
	}
}

func (t *submitJobTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args submitJobArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	if args.JCL == "" && args.Dataset == "" {
		return mcp.ErrorResult("either 'jcl' or 'dataset' must be provided"), nil
	}
	if args.JCL != "" && args.Dataset != "" {
		return mcp.ErrorResult("provide either 'jcl' or 'dataset', not both"), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	var job *zosmf.Job
	if args.JCL != "" {
		job, err = client.SubmitJobJCL(args.JCL)
	} else {
		job, err = client.SubmitJobDataset(args.Dataset)
	}
	if err != nil {
		return mcp.ErrorResult("failed to submit job: " + err.Error()), nil
	}

	data, _ := json.MarshalIndent(job, "", "  ")
	return mcp.TextResult(fmt.Sprintf("Job submitted successfully:\n%s", string(data))), nil
}

// --- Cancel Job ---

type cancelJobTool struct {
	clients *zosmf.ClientRegistry
}

func NewCancelJobTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &cancelJobTool{clients: clients}
}

func (t *cancelJobTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_cancel_job",
		Description: "Cancel a running z/OS job.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":   map[string]any{"type": "string", "description": "Name of the z/OS system"},
				"job_name": map[string]any{"type": "string", "description": "Job name"},
				"job_id":   map[string]any{"type": "string", "description": "Job ID"},
			},
			"required": []string{"system", "job_name", "job_id"},
		}),
	}
}

func (t *cancelJobTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args getJobArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	if err := client.CancelJob(args.JobName, args.JobID); err != nil {
		return mcp.ErrorResult("failed to cancel job: " + err.Error()), nil
	}

	return mcp.TextResult(fmt.Sprintf("Job %s (%s) cancelled successfully", args.JobName, args.JobID)), nil
}

// --- Purge Job ---

type purgeJobTool struct {
	clients *zosmf.ClientRegistry
}

func NewPurgeJobTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &purgeJobTool{clients: clients}
}

func (t *purgeJobTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_purge_job",
		Description: "Purge (delete) a z/OS job's output from the spool.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":   map[string]any{"type": "string", "description": "Name of the z/OS system"},
				"job_name": map[string]any{"type": "string", "description": "Job name"},
				"job_id":   map[string]any{"type": "string", "description": "Job ID"},
			},
			"required": []string{"system", "job_name", "job_id"},
		}),
	}
}

func (t *purgeJobTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args getJobArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	if err := client.PurgeJob(args.JobName, args.JobID); err != nil {
		return mcp.ErrorResult("failed to purge job: " + err.Error()), nil
	}

	return mcp.TextResult(fmt.Sprintf("Job %s (%s) purged successfully", args.JobName, args.JobID)), nil
}

// --- Hold Job ---

type holdJobTool struct {
	clients *zosmf.ClientRegistry
}

func NewHoldJobTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &holdJobTool{clients: clients}
}

func (t *holdJobTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_hold_job",
		Description: "Place a z/OS job on hold.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":   map[string]any{"type": "string", "description": "Name of the z/OS system"},
				"job_name": map[string]any{"type": "string", "description": "Job name"},
				"job_id":   map[string]any{"type": "string", "description": "Job ID"},
			},
			"required": []string{"system", "job_name", "job_id"},
		}),
	}
}

func (t *holdJobTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args getJobArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	if err := client.HoldJob(args.JobName, args.JobID); err != nil {
		return mcp.ErrorResult("failed to hold job: " + err.Error()), nil
	}

	return mcp.TextResult(fmt.Sprintf("Job %s (%s) placed on hold", args.JobName, args.JobID)), nil
}

// --- Release Job ---

type releaseJobTool struct {
	clients *zosmf.ClientRegistry
}

func NewReleaseJobTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &releaseJobTool{clients: clients}
}

func (t *releaseJobTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_release_job",
		Description: "Release a held z/OS job.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":   map[string]any{"type": "string", "description": "Name of the z/OS system"},
				"job_name": map[string]any{"type": "string", "description": "Job name"},
				"job_id":   map[string]any{"type": "string", "description": "Job ID"},
			},
			"required": []string{"system", "job_name", "job_id"},
		}),
	}
}

func (t *releaseJobTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args getJobArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	if err := client.ReleaseJob(args.JobName, args.JobID); err != nil {
		return mcp.ErrorResult("failed to release job: " + err.Error()), nil
	}

	return mcp.TextResult(fmt.Sprintf("Job %s (%s) released", args.JobName, args.JobID)), nil
}

// --- Helpers ---

func mustJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
