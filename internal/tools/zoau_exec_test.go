package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

// stubSSH puts a fake `ssh` first on PATH that records its arguments and
// replays the given output. The ZOAU tools shell out to ssh, so this is what
// lets them run end to end without a z/OS system.
func stubSSH(t *testing.T, stdout, stderr string, exitCode int) (argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")

	script := fmt.Sprintf(`#!/bin/sh
: > %[1]q
for a in "$@"; do printf '%%s\n' "$a" >> %[1]q; done
printf '%%b' %[2]q
printf '%%b' %[3]q >&2
exit %[4]d
`, argsFile, stdout, stderr, exitCode)

	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsFile
}

func testSSHRegistry() *zosmf.SSHClientRegistry {
	return zosmf.NewSSHClientRegistry([]config.SSHSystemConfig{{
		Name: "MAINT", Host: "zos1.example.com", Port: 22, Username: "ibmuser",
	}})
}

func remoteCommand(t *testing.T, argsFile string) string {
	t.Helper()
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("ssh was never called: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	return lines[len(lines)-1]
}

func execTool(t *testing.T, tool mcp.ToolHandler, args string) mcp.ToolCallResult {
	t.Helper()
	res, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute returned a transport error: %v", err)
	}
	return res
}

func resultText(res mcp.ToolCallResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		sb.WriteString(c.Text)
	}
	return sb.String()
}

func TestZOAUToolSendsItsCommand(t *testing.T) {
	argsFile := stubSSH(t, "LNKLST", "", 0)

	res := execTool(t, NewZOAUPllTool(testSSHRegistry()), `{"ssh_system":"MAINT"}`)

	if res.IsError {
		t.Fatalf("unexpected error result: %s", resultText(res))
	}
	if got := remoteCommand(t, argsFile); got != "pll -v" {
		t.Errorf("remote command = %q, want pll -v", got)
	}
	if resultText(res) != "LNKLST" {
		t.Errorf("result = %q, want the command output", resultText(res))
	}
}

// A pattern reaches the remote shell, so it has to be quoted — otherwise a
// value with a space or a semicolon changes the command.
func TestZOAUParmgrepQuotesThePattern(t *testing.T) {
	argsFile := stubSSH(t, "", "", 0)

	execTool(t, NewZOAUParmgrepTool(testSSHRegistry()), `{"ssh_system":"MAINT","pattern":"; rm -rf /"}`)

	got := remoteCommand(t, argsFile)
	if got != `parmgrep -n -v '; rm -rf /'` {
		t.Errorf("remote command = %q, want the pattern in single quotes", got)
	}
}

func TestZOAUExecQuotesCommandAndArgs(t *testing.T) {
	argsFile := stubSSH(t, `{"ok":true}`, "", 0)

	execTool(t, NewZOAUExecTool(testSSHRegistry()),
		`{"ssh_system":"MAINT","command":"/u/user/my script.sh","args":["one two","it's"]}`)

	got := remoteCommand(t, argsFile)
	want := `'/u/user/my script.sh' 'one two' 'it'\''s'`
	if got != want {
		t.Errorf("remote command = %q, want %q", got, want)
	}
}

// zoau_exec reports failures as data — the caller wants the exit code and
// stderr of the remote command, not a transport error.
func TestZOAUExecReturnsExitCodeAndStderr(t *testing.T) {
	stubSSH(t, "", "no such file", 2)

	res := execTool(t, NewZOAUExecTool(testSSHRegistry()),
		`{"ssh_system":"MAINT","command":"cat /u/nope"}`)

	var got struct {
		Stderr   string `json:"stderr"`
		ExitCode int    `json:"exit_code"`
	}
	if err := json.Unmarshal([]byte(resultText(res)), &got); err != nil {
		t.Fatalf("result is not the documented JSON: %v (%s)", err, resultText(res))
	}
	if got.ExitCode != 2 {
		t.Errorf("exit_code = %d, want 2", got.ExitCode)
	}
	if got.Stderr != "no such file" {
		t.Errorf("stderr = %q", got.Stderr)
	}
}

func TestZOAUToolRejectsUnknownSystem(t *testing.T) {
	stubSSH(t, "", "", 0)

	res := execTool(t, NewZOAUZinfoTool(testSSHRegistry()), `{"ssh_system":"NOPE"}`)

	if !res.IsError {
		t.Fatal("an unknown ssh_system must produce an error result")
	}
	if !strings.Contains(resultText(res), "SSH system not found: NOPE") {
		t.Errorf("result = %q", resultText(res))
	}
}

// The system name is matched case-insensitively, as everywhere else.
func TestZOAUToolAcceptsLowercaseSystemName(t *testing.T) {
	stubSSH(t, "ok", "", 0)

	res := execTool(t, NewZOAUZinfoTool(testSSHRegistry()), `{"ssh_system":"maint"}`)

	if res.IsError {
		t.Errorf("lowercase system name was rejected: %s", resultText(res))
	}
}

func TestZOAUToolSurfacesRemoteFailure(t *testing.T) {
	stubSSH(t, "", "zinfo: not found", 127)

	res := execTool(t, NewZOAUZinfoTool(testSSHRegistry()), `{"ssh_system":"MAINT"}`)

	if !res.IsError {
		t.Fatal("a failing command must produce an error result")
	}
	if !strings.Contains(resultText(res), "ZOAU command not found") {
		t.Errorf("result = %q, want the ZOAU hint", resultText(res))
	}
}

func TestZOAUToolRejectsMalformedArguments(t *testing.T) {
	stubSSH(t, "", "", 0)

	res := execTool(t, NewZOAUZinfoTool(testSSHRegistry()), `{"ssh_system":`)

	if !res.IsError {
		t.Fatal("malformed arguments must produce an error result")
	}
	if !strings.Contains(resultText(res), "invalid arguments") {
		t.Errorf("result = %q", resultText(res))
	}
}

// The schema advertises the configured systems so a client can offer them;
// an empty registry must not produce a tool that claims none are needed.
func TestZOAUSchemaListsConfiguredSystems(t *testing.T) {
	def := NewZOAUZinfoTool(testSSHRegistry()).Definition()

	var schema struct {
		Properties struct {
			SSHSystem struct {
				Enum []string `json:"enum"`
			} `json:"ssh_system"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(def.InputSchema, &schema); err != nil {
		t.Fatalf("input schema is not valid JSON: %v", err)
	}
	if len(schema.Properties.SSHSystem.Enum) != 1 || schema.Properties.SSHSystem.Enum[0] != "MAINT" {
		t.Errorf("enum = %v, want [MAINT]", schema.Properties.SSHSystem.Enum)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "ssh_system" {
		t.Errorf("required = %v, want [ssh_system]", schema.Required)
	}
}
