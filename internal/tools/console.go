package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

// --- Issue Command ---

type issueCommandTool struct {
	clients *zosmf.ClientRegistry
}

type issueCommandArgs struct {
	System  string `json:"system"`
	Command string `json:"command"`
	Target  string `json:"target,omitempty"`
}

func NewIssueCommandTool(clients *zosmf.ClientRegistry, cfg interface{}) mcp.ToolHandler {
	return &issueCommandTool{clients: clients}
}

func (t *issueCommandTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_issue_command",
		Description: "Issue a z/OS operator command and return the response. " +
			"Examples: 'D A,L' (display active jobs), 'D T' (display time), " +
			"'D M=CPU' (display CPU), 'S jobname' (start STC), 'P jobname' (stop STC). " +
			"The command is issued synchronously and the response is returned directly. " +
			"WARNING: This is an unrestricted MVS operator console command. Commands such as 'P jobname', " +
			"'C jobname' or 'V device,OFFLINE' change or stop production work, and there is no confirmation " +
			"step and no undo.",
		Annotations: writeTool("Issue Operator Command", true, false, true),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": systemProperty(t.clients.SystemNames()),
				"command": map[string]any{
					"type":        "string",
					"description": "z/OS operator command to issue (e.g. 'D A,L', 'D T', 'S MYJOB')",
				},
				"target": map[string]any{
					"type":        "string",
					"description": "Target system in sysplex for the command (optional, defaults to local system)",
				},
			},
			"required": []string{"system", "command"},
		}),
	}
}

func (t *issueCommandTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args issueCommandArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	// Use "defcn" as default console name (z/OSMF default)
	resp, err := client.IssueCommand("defcn", args.Command, args.Target)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("command failed: %v", err)), nil
	}

	if resp.CmdResponse == "" {
		return mcp.TextResult(fmt.Sprintf("Command '%s' issued successfully (no output)", args.Command)), nil
	}

	return mcp.TextResult(resp.CmdResponse), nil
}

// --- Get Hardcopy Log ---

type getHardcopyLogTool struct {
	clients *zosmf.ClientRegistry
}

type getHardcopyLogArgs struct {
	System    string `json:"system"`
	Source    string `json:"source"`
	Ago       string `json:"ago,omitempty"`
	Since     string `json:"since,omitempty"`
	TimeRange string `json:"time_range,omitempty"`
	Direction string `json:"direction,omitempty"`
	SysName   string `json:"sysname,omitempty"`
	Text      bool   `json:"text,omitempty"`
}

func NewGetHardcopyLogTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &getHardcopyLogTool{clients: clients}
}

func (t *getHardcopyLogTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_get_hardcopy_log",
		Description: "Retrieve z/OS hardcopy log messages from OPERLOG or SYSLOG. " +
			"Use 'ago' for relative time ranges (e.g. '10m', '1h', '30s', '1h30m'). " +
			"Use 'since' for an absolute start time. " +
			"Use 'time_range' to set the window size (default '10m'). " +
			"Use 'direction' to control whether to look forward or backward from the start time.",
		Annotations: readOnlyTool("Read Hardcopy Log"),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": systemProperty(t.clients.SystemNames()),
				"source": map[string]any{
					"type":        "string",
					"description": "Log source: 'operlog' (sysplex-wide) or 'syslog' (single system)",
					"enum":        []string{"operlog", "syslog"},
				},
				"ago": map[string]any{
					"type":        "string",
					"description": "Start time relative to now: '10m', '1h', '30s', '1h30m'. Takes priority over 'since'.",
				},
				"since": map[string]any{
					"type":        "string",
					"description": "Absolute start time in RFC3339 format (e.g. '2026-04-06T04:00:00Z'). Used when 'ago' is not set.",
				},
				"time_range": map[string]any{
					"type":        "string",
					"description": "Time window size: '10m', '1h', '30s'. Defaults to '10m'.",
				},
				"direction": map[string]any{
					"type":        "string",
					"description": "Direction from start time: 'forward' or 'backward'. Defaults to 'backward'.",
					"enum":        []string{"forward", "backward"},
				},
				"sysname": map[string]any{
					"type":        "string",
					"description": "System name filter (only for syslog, optional)",
				},
				"text": map[string]any{
					"type":        "boolean",
					"description": "If true, return messages as formatted plain text instead of JSON.",
				},
			},
			"required": []string{"system", "source"},
		}),
	}
}

func (t *getHardcopyLogTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args getHardcopyLogArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	var since time.Time
	if args.Since != "" && args.Ago == "" {
		since, err = time.Parse(time.RFC3339, args.Since)
		if err != nil {
			return mcp.ErrorResult(fmt.Sprintf("invalid 'since' format, use RFC3339 (e.g. 2026-04-06T04:00:00Z): %v", err)), nil
		}
	}

	resp, err := client.GetHardcopyLog(args.Source, args.Ago, since, args.TimeRange, args.Direction, args.SysName)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("failed to get hardcopy log: %v", err)), nil
	}

	if args.Text {
		var sb strings.Builder
		fmt.Fprintf(&sb, "Source: %s | Total: %d\n\n", resp.Source, resp.TotalItems)
		for _, item := range resp.Items {
			parts := strings.Split(item.Message, "\r")
			var cleaned []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					cleaned = append(cleaned, p)
				}
			}
			msg := strings.Join(cleaned, " ")
			fmt.Fprintf(&sb, "%s %-8s %s\n", item.Time, strings.TrimSpace(item.JobName), msg)
		}
		return mcp.TextResult(sb.String()), nil
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return mcp.TextResult(string(data)), nil
}
