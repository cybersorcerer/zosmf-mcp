package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

type zoauArgs struct {
	SSHSystem string `json:"ssh_system"`
}

type zoauMemberArgs struct {
	SSHSystem string `json:"ssh_system"`
	Member    string `json:"member"`
}

type zoauPatternArgs struct {
	SSHSystem string `json:"ssh_system"`
	Pattern   string `json:"pattern"`
}

func runZOAU(sshReg *zosmf.SSHClientRegistry, system, command string) (mcp.ToolCallResult, error) {
	cfg, err := sshReg.Get(system)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}
	out, err := zosmf.RunSSH(cfg, command)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}
	return mcp.TextResult(out), nil
}

// --- zoau_pll ---

type zoauPllTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUPllTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauPllTool{sshReg: sshReg}
}

func (t *zoauPllTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zoau_pll",
		Description: "Display the z/OS linklist (LLA) concatenation using ZOAU pll -v. Requires ZOAU >= 1.3.x and SSH access.",
		Annotations: readOnlyTool("Show Linklist"),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ssh_system": sshSystemProperty(t.sshReg.SystemNames()),
			},
			"required": []string{"ssh_system"},
		}),
	}
}

func (t *zoauPllTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.SSHSystem, "pll -v")
}

// --- zoau_zinfo ---

type zoauZinfoTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUZinfoTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauZinfoTool{sshReg: sshReg}
}

func (t *zoauZinfoTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zoau_zinfo",
		Description: "Display z/OS system information using ZOAU zinfo -a. Requires ZOAU >= 1.3.x and SSH access.",
		Annotations: readOnlyTool("Show System Information"),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ssh_system": sshSystemProperty(t.sshReg.SystemNames()),
			},
			"required": []string{"ssh_system"},
		}),
	}
}

func (t *zoauZinfoTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.SSHSystem, "zinfo -a")
}

// --- zoau_llwhence ---

type zoauLlwhenceTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAULlwhenceTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauLlwhenceTool{sshReg: sshReg}
}

func (t *zoauLlwhenceTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zoau_llwhence",
		Description: "Locate which linklist dataset a member comes from using ZOAU llwhence -v. Requires ZOAU >= 1.3.x and SSH access.",
		Annotations: readOnlyTool("Locate Linklist Member"),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ssh_system": sshSystemProperty(t.sshReg.SystemNames()),
				"member":     map[string]any{"type": "string", "description": "Member name to locate"},
			},
			"required": []string{"ssh_system", "member"},
		}),
	}
}

func (t *zoauLlwhenceTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauMemberArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.SSHSystem, fmt.Sprintf("llwhence -v %s", zosmf.ShellQuote(args.Member)))
}

// --- zoau_parmgrep ---

type zoauParmgrepTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUParmgrepTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauParmgrepTool{sshReg: sshReg}
}

func (t *zoauParmgrepTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zoau_parmgrep",
		Description: "Search parmlib members for a pattern using ZOAU parmgrep -n -v. Requires ZOAU >= 1.3.x and SSH access.",
		Annotations: readOnlyTool("Search Parmlib"),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ssh_system": sshSystemProperty(t.sshReg.SystemNames()),
				"pattern":    map[string]any{"type": "string", "description": "Search pattern"},
			},
			"required": []string{"ssh_system", "pattern"},
		}),
	}
}

func (t *zoauParmgrepTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauPatternArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.SSHSystem, fmt.Sprintf("parmgrep -n -v %s", zosmf.ShellQuote(args.Pattern)))
}

// --- zoau_parmwhence ---

type zoauParmwhenceTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUParmwhenceTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauParmwhenceTool{sshReg: sshReg}
}

func (t *zoauParmwhenceTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zoau_parmwhence",
		Description: "Locate which parmlib dataset a member comes from using ZOAU parmwhence -v. Requires ZOAU >= 1.3.x and SSH access.",
		Annotations: readOnlyTool("Locate Parmlib Member"),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ssh_system": sshSystemProperty(t.sshReg.SystemNames()),
				"member":     map[string]any{"type": "string", "description": "Parmlib member name to locate"},
			},
			"required": []string{"ssh_system", "member"},
		}),
	}
}

func (t *zoauParmwhenceTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauMemberArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.SSHSystem, fmt.Sprintf("parmwhence -v %s", zosmf.ShellQuote(args.Member)))
}

// --- zoau_pproc ---

type zoauPprocTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUPprocTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauPprocTool{sshReg: sshReg}
}

func (t *zoauPprocTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zoau_pproc",
		Description: "Display the z/OS proclib concatenation using ZOAU pproc -v. Requires ZOAU >= 1.3.x and SSH access.",
		Annotations: readOnlyTool("Show Proclib"),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ssh_system": sshSystemProperty(t.sshReg.SystemNames()),
			},
			"required": []string{"ssh_system"},
		}),
	}
}

func (t *zoauPprocTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.SSHSystem, "pproc -v")
}

// --- zoau_procgrep ---

type zoauProcgrepTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUProcgrepTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauProcgrepTool{sshReg: sshReg}
}

func (t *zoauProcgrepTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zoau_procgrep",
		Description: "Search proclib members for a pattern using ZOAU procgrep -n -v. Requires ZOAU >= 1.3.x and SSH access.",
		Annotations: readOnlyTool("Search Proclib"),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ssh_system": sshSystemProperty(t.sshReg.SystemNames()),
				"pattern":    map[string]any{"type": "string", "description": "Search pattern"},
			},
			"required": []string{"ssh_system", "pattern"},
		}),
	}
}

func (t *zoauProcgrepTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauPatternArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.SSHSystem, fmt.Sprintf("procgrep -n -v %s", zosmf.ShellQuote(args.Pattern)))
}

// --- zoau_procwhence ---

type zoauProcwhenceTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUProcwhenceTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauProcwhenceTool{sshReg: sshReg}
}

func (t *zoauProcwhenceTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zoau_procwhence",
		Description: "Locate which proclib dataset a member comes from using ZOAU procwhence -v. Requires ZOAU >= 1.3.x and SSH access.",
		Annotations: readOnlyTool("Locate Proclib Member"),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ssh_system": sshSystemProperty(t.sshReg.SystemNames()),
				"member":     map[string]any{"type": "string", "description": "Proclib member name to locate"},
			},
			"required": []string{"ssh_system", "member"},
		}),
	}
}

func (t *zoauProcwhenceTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauMemberArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.SSHSystem, fmt.Sprintf("procwhence -v %s", zosmf.ShellQuote(args.Member)))
}

// --- zoau_vtocls ---

type zoauVtoclsArgs struct {
	SSHSystem string   `json:"ssh_system"`
	Volume    string   `json:"volume"`
	Datasets  []string `json:"datasets,omitempty"`
	Usage     bool     `json:"usage,omitempty"`
}

type zoauVtoclsTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUVtoclsTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauVtoclsTool{sshReg: sshReg}
}

func (t *zoauVtoclsTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zoau_vtocls",
		Description: "List the VTOC contents of a z/OS volume using ZOAU vtocls. Optionally filter by dataset names and include last reference time. Requires ZOAU >= 1.3.x and SSH access.",
		Annotations: readOnlyTool("List Volume VTOC"),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ssh_system": sshSystemProperty(t.sshReg.SystemNames()),
				"volume":     map[string]any{"type": "string", "description": "Volume serial (VOLSER) to list, e.g. 'USER10'"},
				"datasets":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional list of dataset name filters"},
				"usage":      map[string]any{"type": "boolean", "description": "Include last reference time in output"},
			},
			"required": []string{"ssh_system", "volume"},
		}),
	}
}

func (t *zoauVtoclsTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauVtoclsArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	cmd := "vtocls"
	if args.Usage {
		cmd += " -u"
	}
	cmd += " " + zosmf.ShellQuote(args.Volume)
	for _, ds := range args.Datasets {
		cmd += " " + zosmf.ShellQuote(ds)
	}
	return runZOAU(t.sshReg, args.SSHSystem, cmd)
}

// --- zoau_exec ---

type zoauExecArgs struct {
	SSHSystem string   `json:"ssh_system"`
	Command   string   `json:"command"`
	Args      []string `json:"args,omitempty"`
	Timeout   int      `json:"timeout,omitempty"`
}

type zoauExecTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUExecTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauExecTool{sshReg: sshReg}
}

func (t *zoauExecTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zoau_exec",
		Description: "Execute an arbitrary command or script on a z/OS USS system over SSH. " +
			"Returns stdout, stderr and the exit code as JSON. Suitable for REXX scripts, shell scripts, " +
			"Python scripts, and any USS command. " +
			"WARNING: This is unrestricted shell access under the configured SSH user's authority — it can " +
			"delete data, submit jobs and change system state, with no confirmation step and no undo. " +
			"Prefer a specific tool when one exists. " +
			"The command string and each entry of args are shell-quoted individually, so a value containing " +
			"spaces or quotes stays one argument; the command itself is not restricted to a whitelist.",
		Annotations: writeTool("Execute USS Command", true, false, true),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ssh_system": sshSystemProperty(t.sshReg.SystemNames()),
				"command":    map[string]any{"type": "string", "description": "Command or script path to execute, e.g. '/u/user/myscript.sh' or 'python3 /u/user/script.py'"},
				"args":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional list of arguments passed to the command"},
				"timeout":    map[string]any{"type": "integer", "description": "Timeout in seconds (default: 30)"},
			},
			"required": []string{"ssh_system", "command"},
		}),
	}
}

func (t *zoauExecTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauExecArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	cfg, err := t.sshReg.Get(args.SSHSystem)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	cmd := zosmf.ShellQuote(args.Command)
	for _, a := range args.Args {
		cmd += " " + zosmf.ShellQuote(a)
	}

	result, err := zosmf.RunSSHDetailed(cfg, cmd, args.Timeout)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	data, _ := json.Marshal(result)
	return mcp.TextResult(string(data)), nil
}

// --- zoau_version ---

type zoauVersionTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUVersionTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauVersionTool{sshReg: sshReg}
}

func (t *zoauVersionTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zoau_version",
		Description: "Display the installed ZOAU version using zoaversion -v. Requires ZOAU >= 1.3.x and SSH access.",
		Annotations: readOnlyTool("Show ZOAU Version"),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ssh_system": sshSystemProperty(t.sshReg.SystemNames()),
			},
			"required": []string{"ssh_system"},
		}),
	}
}

func (t *zoauVersionTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.SSHSystem, "zoaversion -v")
}
