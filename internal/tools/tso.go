package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

type tsoCommandTool struct {
	clients *zosmf.ClientRegistry
	cfg     *config.Config
}

type tsoCommandArgs struct {
	System  string `json:"system"`
	Command string `json:"command"`
}

func NewTSOCommandTool(clients *zosmf.ClientRegistry, cfg *config.Config) mcp.ToolHandler {
	return &tsoCommandTool{clients: clients, cfg: cfg}
}

func (t *tsoCommandTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_tso_command",
		Description: "Execute a TSO/E command on a z/OS system and return the output. " +
			"A TSO session is started automatically, the command is executed, and the session is stopped. " +
			"Examples: 'LISTDS USER.JCL', 'LISTCAT LVL(USER)', 'TRANSMIT USER.JCL(MYJOB)'. " +
			"Requires tso_account and tso_proc to be set for this system in the server configuration; " +
			"without them the call fails before the command runs. " +
			"WARNING: TSO commands are unrestricted — 'DELETE' and 'ALTER' reach real data.",
		Annotations: writeTool("Execute TSO Command", true, false, true),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": systemProperty(t.clients.SystemNames()),
				"command": map[string]any{
					"type":        "string",
					"description": "TSO/E command to execute (e.g. 'LISTDS USER.JCL', 'LISTCAT LVL(USER)')",
				},
			},
			"required": []string{"system", "command"},
		}),
	}
}

func (t *tsoCommandTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args tsoCommandArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	sysCfg, err := t.cfg.SystemByName(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	if sysCfg.TSOAccount == "" {
		return mcp.ErrorResult(fmt.Sprintf("system %s has no tso_account configured", args.System)), nil
	}
	if sysCfg.TSOProc == "" {
		return mcp.ErrorResult(fmt.Sprintf("system %s has no tso_proc configured", args.System)), nil
	}

	output, err := client.RunTSOCommand(args.Command, sysCfg.TSOAccount, sysCfg.TSOProc, sysCfg.TSOCharset, sysCfg.TSOCodepage)
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("TSO command failed: %v", err)), nil
	}

	if output == "" {
		return mcp.TextResult(fmt.Sprintf("Command '%s' executed successfully (no output)", args.Command)), nil
	}

	return mcp.TextResult(output), nil
}
