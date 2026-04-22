# ZOAU Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate 10 ZOAU CLI commands as MCP tools, executed via SSH per LPAR, with a dedicated `ssh_systems` config section.

**Architecture:** A new `SSHClientRegistry` (analogous to the existing `zosmf.ClientRegistry`) resolves LPAR names to SSH configs. A `RunSSH` function executes ZOAU commands via the system `ssh` binary with a 30-second timeout. Ten MCP tool handlers in `internal/tools/zoau.go` use this registry; they are only registered when `ssh_systems` is non-empty in the config.

**Tech Stack:** Go 1.22+, `os/exec` for SSH, `context.WithTimeout` for timeouts, `encoding/json` for optional JSON parsing, `gopkg.in/yaml.v3` for config.

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `internal/config/config.go` | Add `SSHSystemConfig` struct and `SSHSystems []SSHSystemConfig` to `Config` |
| Create | `internal/zosmf/ssh.go` | `SSHClientRegistry`, `RunSSH` |
| Create | `internal/tools/zoau.go` | 10 ZOAU MCP tool handlers |
| Modify | `internal/tools/registry.go` | Conditionally register ZOAU tools |
| Modify | `cmd/zosmf-mcp/main.go` | Build `SSHClientRegistry`, pass to `RegisterAll` |

---

## Task 1: Add `SSHSystemConfig` to config

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Add `SSHSystemConfig` struct and `SSHSystems` field to `Config`**

In `internal/config/config.go`, add after the `HTTPConfig` struct:

```go
type SSHSystemConfig struct {
	Name     string `yaml:"name"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	KeyFile  string `yaml:"key_file"`
}
```

Add `SSHSystems` field to `Config`:

```go
type Config struct {
	Systems    []SystemConfig    `yaml:"systems"`
	Servers    []SystemConfig    `yaml:"servers"`
	SSHSystems []SSHSystemConfig `yaml:"ssh_systems"`
	Transport  TransportConfig   `yaml:"transport"`
}
```

- [ ] **Step 2: Add SSH config normalization — default port 22**

In the `normalize()` method, add after the `for i := range c.Systems` loop:

```go
for i := range c.SSHSystems {
	if c.SSHSystems[i].Port == 0 {
		c.SSHSystems[i].Port = 22
	}
}
```

- [ ] **Step 3: Add SSH config validation**

In the `validate()` method, add after the systems validation loop:

```go
sshNames := make(map[string]bool)
for i, ssh := range c.SSHSystems {
	if ssh.Name == "" {
		return fmt.Errorf("ssh_systems entry %d: name is required", i)
	}
	normalized := strings.ToUpper(ssh.Name)
	if sshNames[normalized] {
		return fmt.Errorf("duplicate ssh_systems name: %s", ssh.Name)
	}
	sshNames[normalized] = true
	if ssh.Host == "" {
		return fmt.Errorf("ssh_systems %s: host is required", ssh.Name)
	}
	if ssh.Username == "" {
		return fmt.Errorf("ssh_systems %s: username is required", ssh.Name)
	}
}
```

- [ ] **Step 4: Build and verify no errors**

```bash
make build
```

Expected: build succeeds with no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go
git commit -m "feat: add SSHSystemConfig to config"
```

---

## Task 2: SSH execution layer

**Files:**
- Create: `internal/zosmf/ssh.go`

- [ ] **Step 1: Create `internal/zosmf/ssh.go`**

```go
package zosmf

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/config"
)

// SSHClientRegistry resolves LPAR names to SSH configurations.
type SSHClientRegistry struct {
	systems map[string]config.SSHSystemConfig
}

// NewSSHClientRegistry builds a registry from the configured SSH systems.
func NewSSHClientRegistry(systems []config.SSHSystemConfig) *SSHClientRegistry {
	m := make(map[string]config.SSHSystemConfig, len(systems))
	for _, s := range systems {
		m[strings.ToUpper(s.Name)] = s
	}
	return &SSHClientRegistry{systems: m}
}

// Get returns the SSH config for the named system (case-insensitive).
func (r *SSHClientRegistry) Get(name string) (config.SSHSystemConfig, error) {
	cfg, ok := r.systems[strings.ToUpper(name)]
	if !ok {
		return config.SSHSystemConfig{}, fmt.Errorf("SSH system not found: %s", name)
	}
	return cfg, nil
}

// RunSSH executes a command on a remote z/OS system via SSH.
// Only key-based authentication is supported (BatchMode=yes).
func RunSSH(cfg config.SSHSystemConfig, command string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := []string{
		"-p", fmt.Sprintf("%d", cfg.Port),
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if cfg.KeyFile != "" {
		args = append(args, "-i", cfg.KeyFile)
	}
	args = append(args, fmt.Sprintf("%s@%s", cfg.Username, cfg.Host), command)

	cmd := exec.CommandContext(ctx, "ssh", args...)
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
			if exitErr.ExitCode() == 127 {
				return "", fmt.Errorf("ZOAU command not found — is ZOAU >= 1.3.x installed and in PATH?")
			}
		}
		if stderr != "" {
			return "", fmt.Errorf("SSH command failed: %s", stderr)
		}
		return "", fmt.Errorf("SSH command failed: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}
```

- [ ] **Step 2: Build and verify**

```bash
make build
```

Expected: build succeeds with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/zosmf/ssh.go
git commit -m "feat: add SSH execution layer for ZOAU"
```

---

## Task 3: ZOAU MCP tool handlers

**Files:**
- Create: `internal/tools/zoau.go`

- [ ] **Step 1: Create `internal/tools/zoau.go` with shared base and first two tools**

```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

// zoauArgs is the common argument struct for ZOAU tools that only need a system name.
type zoauArgs struct {
	System string `json:"system"`
}

// zoauMemberArgs is used by tools that need a system name and a member name.
type zoauMemberArgs struct {
	System string `json:"system"`
	Member string `json:"member"`
}

// zoauPatternArgs is used by tools that need a system name and a search pattern.
type zoauPatternArgs struct {
	System  string `json:"system"`
	Pattern string `json:"pattern"`
}

// runZOAU executes a ZOAU command and returns the output.
// If jsonFlag is true, -j is appended and output is parsed as JSON then
// re-serialized as indented text. Falls back to plain text on parse error.
func runZOAU(sshReg *zosmf.SSHClientRegistry, system, command string, jsonFlag bool) (mcp.ToolCallResult, error) {
	cfg, err := sshReg.Get(system)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}
	cmd := command
	if jsonFlag {
		cmd = command + " -j"
	}
	out, err := zosmf.RunSSH(cfg, cmd)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}
	if jsonFlag {
		var parsed any
		if jsonErr := json.Unmarshal([]byte(out), &parsed); jsonErr == nil {
			pretty, _ := json.MarshalIndent(parsed, "", "  ")
			return mcp.TextResult(string(pretty)), nil
		}
		// fall back to plain text
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
	return runZOAU(t.sshReg, args.System, "pll -v", false)
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
	return runZOAU(t.sshReg, args.System, "zinfo -a", false)
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
	return runZOAU(t.sshReg, args.System, fmt.Sprintf("llwhence -v %s", args.Member), false)
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
	return runZOAU(t.sshReg, args.System, fmt.Sprintf("parmgrep -n -v %s", args.Pattern), false)
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
	return runZOAU(t.sshReg, args.System, fmt.Sprintf("parmwhence -v %s", args.Member), false)
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
	return runZOAU(t.sshReg, args.System, "pproc -v", false)
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
	return runZOAU(t.sshReg, args.System, fmt.Sprintf("procgrep -n -v %s", args.Pattern), false)
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
	return runZOAU(t.sshReg, args.System, fmt.Sprintf("procwhence -v %s", args.Member), false)
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
	return runZOAU(t.sshReg, args.System, "zoaversion -v", false)
}
```

- [ ] **Step 2: Build and verify**

```bash
make build
```

Expected: build succeeds with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/tools/zoau.go
git commit -m "feat: add ZOAU MCP tool handlers"
```

---

## Task 4: Conditionally register ZOAU tools

**Files:**
- Modify: `internal/tools/registry.go`
- Modify: `cmd/zosmf-mcp/main.go`

- [ ] **Step 1: Update `RegisterAll` signature to accept `SSHClientRegistry`**

Replace the `RegisterAll` function signature in `internal/tools/registry.go`:

```go
package tools

import (
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/config"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

// RegisterAll registers all available MCP tools with the given registry.
// ZOAU tools are only registered if sshReg is non-nil (i.e. ssh_systems is configured).
func RegisterAll(registry *mcp.ToolRegistry, clients *zosmf.ClientRegistry, cfg *config.Config, sshReg *zosmf.SSHClientRegistry) error {
	handlers := []mcp.ToolHandler{
		// Jobs tools
		NewListJobsTool(clients),
		NewGetJobTool(clients),
		NewGetJobOutputTool(clients),
		NewGetSpoolContentTool(clients),
		NewSubmitJobTool(clients),
		NewCancelJobTool(clients),
		NewPurgeJobTool(clients),
		NewHoldJobTool(clients),
		NewReleaseJobTool(clients),
		// SMP/E tools
		NewQueryCSITool(clients, cfg),
		// Dataset tools
		NewListDatasetsTool(clients),
		NewListMembersTool(clients),
		NewReadDatasetTool(clients),
		NewWriteDatasetTool(clients),
		NewAcquireDatasetEnqTool(clients),
		NewReleaseDatasetEnqTool(clients),
		// Filesystem tools
		NewListFilesystemsTool(clients),
		NewMountFilesystemTool(clients),
		NewUnmountFilesystemTool(clients),
		NewCreateZFSTool(clients),
		NewDeleteZFSTool(clients),
		// Console tools
		NewIssueCommandTool(clients, nil),
		NewGetHardcopyLogTool(clients),
		// USS Files tools
		NewListUSSFilesTool(clients),
		NewReadUSSFileTool(clients),
		NewWriteUSSFileTool(clients),
		NewCreateUSSFileTool(clients),
		NewDeleteUSSFileTool(clients),
		// Extended Dataset tools
		NewCreateDatasetTool(clients),
		NewDeleteDatasetTool(clients),
		// TSO tool
		NewTSOCommandTool(clients, cfg),
		// Parmlib tools
		NewParmlibValidateTool(clients),
	}

	// ZOAU tools — only registered when ssh_systems is configured
	if sshReg != nil {
		handlers = append(handlers,
			NewZOAUPllTool(sshReg),
			NewZOAUZinfoTool(sshReg),
			NewZOAULlwhenceTool(sshReg),
			NewZOAUParmgrepTool(sshReg),
			NewZOAUParmwhenceTool(sshReg),
			NewZOAUPprocTool(sshReg),
			NewZOAUProcgrepTool(sshReg),
			NewZOAUProcwhenceTool(sshReg),
			NewZOAUVersionTool(sshReg),
		)
	}

	for _, h := range handlers {
		if err := registry.Register(h); err != nil {
			return err
		}
	}

	return nil
}
```

- [ ] **Step 2: Update `main.go` to build `SSHClientRegistry` and pass it to `RegisterAll`**

In `cmd/zosmf-mcp/main.go`, replace the `RegisterAll` call block:

```go
// Build SSH client registry if ssh_systems are configured
var sshReg *zosmf.SSHClientRegistry
if len(cfg.SSHSystems) > 0 {
	sshReg = zosmf.NewSSHClientRegistry(cfg.SSHSystems)
	logger.Printf("registered %d SSH systems", len(cfg.SSHSystems))
}

// Create tool registry and register all tools
registry := mcp.NewToolRegistry()
if err := tools.RegisterAll(registry, clients, cfg, sshReg); err != nil {
	logger.Fatalf("failed to register tools: %v", err)
}
logger.Printf("registered %d tools", len(registry.List()))
```

- [ ] **Step 3: Build and verify**

```bash
make build
```

Expected: build succeeds with no errors.

- [ ] **Step 4: Smoke test — start server with existing config (no ssh_systems)**

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' \
  | bin/zosmf-mcp --config .zosmf-mcp-config.yaml
```

Expected: server responds with `InitializeResult`, no ZOAU tools in the tool count (still 32).

- [ ] **Step 5: Smoke test — start server with ssh_systems in config**

Add a temporary `ssh_systems` entry to `.zosmf-mcp-config.yaml`:

```yaml
ssh_systems:
  - name: "SYSA"
    host: "203.0.113.11"
    port: 22
    username: "ibmuser"
```

Then:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' \
  | bin/zosmf-mcp --config .zosmf-mcp-config.yaml 2>/dev/null \
  | python3 -c "import sys,json; tools=json.load(sys.stdin)['result']['tools']; print(len(tools), 'tools'); [print(t['name']) for t in tools if 'zoau' in t['name']]"
```

Expected: 41 tools total, 9 ZOAU tools listed.

- [ ] **Step 6: Commit**

```bash
git add internal/tools/registry.go cmd/zosmf-mcp/main.go
git commit -m "feat: conditionally register ZOAU tools when ssh_systems configured"
```

---

## Task 5: Install and update documentation

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Install updated binary**

```bash
make install
```

Expected: binary installed to `~/.local/bin/zosmf-mcp`.

- [ ] **Step 2: Add `ssh_systems` section to README**

In `README.md`, in section **3. Configuration**, after the existing config example, add a new subsection:

```markdown
### SSH Systems (for ZOAU tools)

ZOAU tools require SSH access to individual LPARs. SSH systems are configured independently of z/OSMF systems — one z/OSMF endpoint can serve an entire sysplex, while SSH requires a separate entry per LPAR.

```yaml
ssh_systems:
  - name: "MVSA"
    host: "mvsa.example.com"
    port: 22                    # optional, default: 22
    username: "ibmuser"
    key_file: "~/.ssh/id_rsa"  # optional — omit to use SSH default key
  - name: "MVSB"
    host: "mvsb.example.com"
    username: "ibmuser"
```

ZOAU tools are only registered when at least one `ssh_systems` entry is present. Password-based SSH is not supported — only key authentication.
```

- [ ] **Step 3: Add ZOAU tools to Tools Overview table in README**

In section **5. Tools Overview**, add a new category after Parmlib:

```markdown
### ZOAU (9 tools) — requires `ssh_systems` configuration

| Tool | Description |
|------|-------------|
| `zosmf_zoau_pll` | Display the z/OS linklist (LLA) concatenation |
| `zosmf_zoau_zinfo` | Display z/OS system information |
| `zosmf_zoau_llwhence` | Locate which linklist dataset a member comes from |
| `zosmf_zoau_parmgrep` | Search parmlib members for a pattern |
| `zosmf_zoau_parmwhence` | Locate which parmlib dataset a member comes from |
| `zosmf_zoau_pproc` | Display the z/OS proclib concatenation |
| `zosmf_zoau_procgrep` | Search proclib members for a pattern |
| `zosmf_zoau_procwhence` | Locate which proclib dataset a member comes from |
| `zosmf_zoau_version` | Display the installed ZOAU version |
```

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add ZOAU ssh_systems config and tool overview to README"
```
