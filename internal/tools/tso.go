package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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
			"If the system has tso_account in the server configuration, the command runs in its own TSO address space, " +
			"started with that account number and the configured tso_proc (default IZUFPROC) and stopped afterwards; " +
			"otherwise z/OSMF runs it in an address space it sets up itself. " +
			"Examples: 'LISTDS USER.JCL', 'LISTCAT LVL(USER)', 'TRANSMIT USER.JCL(MYJOB)'. " +
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

	// Procedure, character set and code page only reach z/OSMF together with
	// an account number. Running without one would ignore them silently.
	if sysCfg.TSOAccount == "" {
		var unused []string
		for key, value := range map[string]string{"tso_proc": sysCfg.TSOProc, "tso_charset": sysCfg.TSOCharset, "tso_codepage": sysCfg.TSOCodepage} {
			if value != "" {
				unused = append(unused, key)
			}
		}
		if len(unused) > 0 {
			sort.Strings(unused)
			return mcp.ErrorResult(fmt.Sprintf("system %s sets %s but no tso_account — they only take effect together with an account number",
				args.System, strings.Join(unused, ", "))), nil
		}
	}

	output, err := client.RunTSOCommand(args.Command, zosmf.TSOAddressSpace{
		Account:  sysCfg.TSOAccount,
		Proc:     sysCfg.TSOProc,
		Charset:  sysCfg.TSOCharset,
		Codepage: sysCfg.TSOCodepage,
	})
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("TSO command failed: %v", err)), nil
	}

	if output == "" {
		return mcp.TextResult(fmt.Sprintf("Command '%s' executed successfully (no output)", args.Command)), nil
	}

	return mcp.TextResult(output), nil
}
