package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

type zoauArgs struct {
	System string `json:"system"`
}

type zoauMemberArgs struct {
	System string `json:"system"`
	Member string `json:"member"`
}

type zoauPatternArgs struct {
	System  string `json:"system"`
	Pattern string `json:"pattern"`
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

// --- zosmf_zoau_pll ---

type zoauPllTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUPllTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauPllTool{sshReg: sshReg}
}

func (t *zoauPllTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_zoau_pll",
		Description: "Display the z/OS linklist (LLA) concatenation using ZOAU pll -v. Requires ZOAU >= 1.3.x and SSH access.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{"type": "string", "description": "LPAR name as defined in ssh_systems"},
			},
			"required": []string{"system"},
		}),
	}
}

func (t *zoauPllTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.System, "pll -v")
}

// --- zosmf_zoau_zinfo ---

type zoauZinfoTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUZinfoTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauZinfoTool{sshReg: sshReg}
}

func (t *zoauZinfoTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_zoau_zinfo",
		Description: "Display z/OS system information using ZOAU zinfo -a. Requires ZOAU >= 1.3.x and SSH access.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{"type": "string", "description": "LPAR name as defined in ssh_systems"},
			},
			"required": []string{"system"},
		}),
	}
}

func (t *zoauZinfoTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.System, "zinfo -a")
}

// --- zosmf_zoau_llwhence ---

type zoauLlwhenceTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAULlwhenceTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauLlwhenceTool{sshReg: sshReg}
}

func (t *zoauLlwhenceTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_zoau_llwhence",
		Description: "Locate which linklist dataset a member comes from using ZOAU llwhence -v. Requires ZOAU >= 1.3.x and SSH access.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{"type": "string", "description": "LPAR name as defined in ssh_systems"},
				"member": map[string]any{"type": "string", "description": "Member name to locate"},
			},
			"required": []string{"system", "member"},
		}),
	}
}

func (t *zoauLlwhenceTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauMemberArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.System, fmt.Sprintf("llwhence -v %s", zosmf.ShellQuote(args.Member)))
}

// --- zosmf_zoau_parmgrep ---

type zoauParmgrepTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUParmgrepTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauParmgrepTool{sshReg: sshReg}
}

func (t *zoauParmgrepTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_zoau_parmgrep",
		Description: "Search parmlib members for a pattern using ZOAU parmgrep -n -v. Requires ZOAU >= 1.3.x and SSH access.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":  map[string]any{"type": "string", "description": "LPAR name as defined in ssh_systems"},
				"pattern": map[string]any{"type": "string", "description": "Search pattern"},
			},
			"required": []string{"system", "pattern"},
		}),
	}
}

func (t *zoauParmgrepTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauPatternArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.System, fmt.Sprintf("parmgrep -n -v %s", zosmf.ShellQuote(args.Pattern)))
}

// --- zosmf_zoau_parmwhence ---

type zoauParmwhenceTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUParmwhenceTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauParmwhenceTool{sshReg: sshReg}
}

func (t *zoauParmwhenceTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_zoau_parmwhence",
		Description: "Locate which parmlib dataset a member comes from using ZOAU parmwhence -v. Requires ZOAU >= 1.3.x and SSH access.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{"type": "string", "description": "LPAR name as defined in ssh_systems"},
				"member": map[string]any{"type": "string", "description": "Parmlib member name to locate"},
			},
			"required": []string{"system", "member"},
		}),
	}
}

func (t *zoauParmwhenceTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauMemberArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.System, fmt.Sprintf("parmwhence -v %s", zosmf.ShellQuote(args.Member)))
}

// --- zosmf_zoau_pproc ---

type zoauPprocTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUPprocTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauPprocTool{sshReg: sshReg}
}

func (t *zoauPprocTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_zoau_pproc",
		Description: "Display the z/OS proclib concatenation using ZOAU pproc -v. Requires ZOAU >= 1.3.x and SSH access.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{"type": "string", "description": "LPAR name as defined in ssh_systems"},
			},
			"required": []string{"system"},
		}),
	}
}

func (t *zoauPprocTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.System, "pproc -v")
}

// --- zosmf_zoau_procgrep ---

type zoauProcgrepTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUProcgrepTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauProcgrepTool{sshReg: sshReg}
}

func (t *zoauProcgrepTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_zoau_procgrep",
		Description: "Search proclib members for a pattern using ZOAU procgrep -n -v. Requires ZOAU >= 1.3.x and SSH access.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":  map[string]any{"type": "string", "description": "LPAR name as defined in ssh_systems"},
				"pattern": map[string]any{"type": "string", "description": "Search pattern"},
			},
			"required": []string{"system", "pattern"},
		}),
	}
}

func (t *zoauProcgrepTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauPatternArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.System, fmt.Sprintf("procgrep -n -v %s", zosmf.ShellQuote(args.Pattern)))
}

// --- zosmf_zoau_procwhence ---

type zoauProcwhenceTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUProcwhenceTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauProcwhenceTool{sshReg: sshReg}
}

func (t *zoauProcwhenceTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_zoau_procwhence",
		Description: "Locate which proclib dataset a member comes from using ZOAU procwhence -v. Requires ZOAU >= 1.3.x and SSH access.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{"type": "string", "description": "LPAR name as defined in ssh_systems"},
				"member": map[string]any{"type": "string", "description": "Proclib member name to locate"},
			},
			"required": []string{"system", "member"},
		}),
	}
}

func (t *zoauProcwhenceTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauMemberArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.System, fmt.Sprintf("procwhence -v %s", zosmf.ShellQuote(args.Member)))
}

// --- zosmf_zoau_vtocls ---

type zoauVtoclsArgs struct {
	System   string   `json:"system"`
	Volume   string   `json:"volume"`
	Datasets []string `json:"datasets,omitempty"`
	Usage    bool     `json:"usage,omitempty"`
}

type zoauVtoclsTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUVtoclsTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauVtoclsTool{sshReg: sshReg}
}

func (t *zoauVtoclsTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_zoau_vtocls",
		Description: "List the VTOC contents of a z/OS volume using ZOAU vtocls. Optionally filter by dataset names and include last reference time. Requires ZOAU >= 1.3.x and SSH access.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":   map[string]any{"type": "string", "description": "LPAR name as defined in ssh_systems"},
				"volume":   map[string]any{"type": "string", "description": "Volume serial (VOLSER) to list, e.g. 'USER10'"},
				"datasets": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional list of dataset name filters"},
				"usage":    map[string]any{"type": "boolean", "description": "Include last reference time in output"},
			},
			"required": []string{"system", "volume"},
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
	return runZOAU(t.sshReg, args.System, cmd)
}

// --- zosmf_zoau_exec ---

type zoauExecArgs struct {
	System  string   `json:"system"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Timeout int      `json:"timeout,omitempty"`
}

type zoauExecTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUExecTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauExecTool{sshReg: sshReg}
}

func (t *zoauExecTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_zoau_exec",
		Description: "Execute an arbitrary command or script on a z/OS USS system via SSH. Returns stdout, stderr and exit code as JSON. Suitable for REXX scripts, shell scripts, Python scripts, and any USS command.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":  map[string]any{"type": "string", "description": "LPAR name as defined in ssh_systems"},
				"command": map[string]any{"type": "string", "description": "Command or script path to execute, e.g. '/u/user/myscript.sh' or 'python3 /u/user/script.py'"},
				"args":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional list of arguments passed to the command"},
				"timeout": map[string]any{"type": "integer", "description": "Timeout in seconds (default: 30)"},
			},
			"required": []string{"system", "command"},
		}),
	}
}

func (t *zoauExecTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauExecArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	cfg, err := t.sshReg.Get(args.System)
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

// --- zosmf_zoau_version ---

type zoauVersionTool struct{ sshReg *zosmf.SSHClientRegistry }

func NewZOAUVersionTool(sshReg *zosmf.SSHClientRegistry) mcp.ToolHandler {
	return &zoauVersionTool{sshReg: sshReg}
}

func (t *zoauVersionTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_zoau_version",
		Description: "Display the installed ZOAU version using zoaversion -v. Requires ZOAU >= 1.3.x and SSH access.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{"type": "string", "description": "LPAR name as defined in ssh_systems"},
			},
			"required": []string{"system"},
		}),
	}
}

func (t *zoauVersionTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args zoauArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	return runZOAU(t.sshReg, args.System, "zoaversion -v")
}
