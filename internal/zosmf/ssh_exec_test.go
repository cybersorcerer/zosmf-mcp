package zosmf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
)

// fakeSSH puts a stub `ssh` first on PATH. The stub records the arguments it
// was called with, then produces the given output and exit code. It lets the
// tests below exercise RunSSH and RunSSHDetailed — the code that actually
// assembles and runs the command — without a z/OS system.
func fakeSSH(t *testing.T, stdout, stderr string, exitCode int) (argsFile string) {
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

func recordedArgs(t *testing.T, argsFile string) []string {
	t.Helper()
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("the stub ssh recorded no arguments: %v", err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func testSSHSystem() config.SSHSystemConfig {
	return config.SSHSystemConfig{
		Name: "SYS1", Host: "zos1.example.com", Port: 22, Username: "ibmuser",
	}
}

func TestRunSSHPassesTargetAndCommand(t *testing.T) {
	argsFile := fakeSSH(t, "", "", 0)

	if _, err := RunSSH(testSSHSystem(), "zinfo -j"); err != nil {
		t.Fatalf("RunSSH: %v", err)
	}

	args := recordedArgs(t, argsFile)
	if got := args[len(args)-2]; got != "ibmuser@zos1.example.com" {
		t.Errorf("target = %q, want ibmuser@zos1.example.com", got)
	}
	if got := args[len(args)-1]; got != "zinfo -j" {
		t.Errorf("remote command = %q, want zinfo -j", got)
	}
}

// ZOAU lives outside the default PATH of a z/OS login shell, so the
// configured environment has to reach the remote side with the command.
func TestRunSSHPrefixesTheConfiguredEnvironment(t *testing.T) {
	argsFile := fakeSSH(t, "", "", 0)

	cfg := testSSHSystem()
	cfg.Environment = map[string]string{
		"ZOAU_HOME": "/var/devops/zoautil",
		"PATH":      "/var/devops/zoautil/bin:/bin",
	}
	if _, err := RunSSH(cfg, "zinfo"); err != nil {
		t.Fatalf("RunSSH: %v", err)
	}

	remote := recordedArgs(t, argsFile)
	last := remote[len(remote)-1]
	for _, want := range []string{
		"export ZOAU_HOME='/var/devops/zoautil'",
		"export PATH='/var/devops/zoautil/bin:/bin'",
	} {
		if !strings.Contains(last, want) {
			t.Errorf("remote command %q lacks %q", last, want)
		}
	}
	if !strings.HasSuffix(last, "&& zinfo") {
		t.Errorf("remote command %q must end with the command itself", last)
	}
}

func TestRunSSHTrimsTrailingNewline(t *testing.T) {
	fakeSSH(t, "line one\nline two\n\n", "", 0)

	out, err := RunSSH(testSSHSystem(), "zinfo")
	if err != nil {
		t.Fatalf("RunSSH: %v", err)
	}
	if out != "line one\nline two" {
		t.Errorf("output = %q, want the trailing newlines trimmed", out)
	}
}

// Exit code 127 from a shell means "command not found". Without the special
// case the caller sees an empty error and no hint that ZOAU is missing.
func TestRunSSHReportsMissingZOAU(t *testing.T) {
	fakeSSH(t, "", "zinfo: not found", 127)

	_, err := RunSSH(testSSHSystem(), "zinfo")
	if err == nil {
		t.Fatal("expected an error for exit code 127")
	}
	if !strings.Contains(err.Error(), "ZOAU command not found") {
		t.Errorf("error = %q, want the ZOAU hint", err)
	}
}

func TestRunSSHSurfacesStderr(t *testing.T) {
	fakeSSH(t, "", "Permission denied, please try again.", 255)

	_, err := RunSSH(testSSHSystem(), "zinfo")
	if err == nil {
		t.Fatal("expected an error for exit code 255")
	}
	if !strings.Contains(err.Error(), "SSH command failed: Permission denied") {
		t.Errorf("error = %q, want the remote stderr passed through", err)
	}
}

func TestRunSSHDetailedReturnsAllThreeChannels(t *testing.T) {
	fakeSSH(t, "the output\n", "the warning\n", 8)

	res, err := RunSSHDetailed(testSSHSystem(), "zinfo", 5)
	if err != nil {
		t.Fatalf("RunSSHDetailed: %v", err)
	}
	if res.Stdout != "the output" {
		t.Errorf("Stdout = %q", res.Stdout)
	}
	if res.Stderr != "the warning" {
		t.Errorf("Stderr = %q", res.Stderr)
	}
	if res.ExitCode != 8 {
		t.Errorf("ExitCode = %d, want 8", res.ExitCode)
	}
}

// A non-zero exit is data for RunSSHDetailed, not a failure: the caller wants
// to see the exit code and stderr of the remote command.
func TestRunSSHDetailedTreatsNonZeroExitAsResult(t *testing.T) {
	fakeSSH(t, "", "no such file", 2)

	res, err := RunSSHDetailed(testSSHSystem(), "cat /u/nope", 5)
	if err != nil {
		t.Fatalf("a non-zero exit must not be an error: %v", err)
	}
	if res.ExitCode != 2 {
		t.Errorf("ExitCode = %d, want 2", res.ExitCode)
	}
}

func TestRunSSHDetailedPassesTheProxyCommand(t *testing.T) {
	argsFile := fakeSSH(t, "", "", 0)

	cfg := testSSHSystem()
	cfg.KeyFile = "/keys/zos1"
	cfg.ProxyJump = &config.SSHProxyJumpConfig{Enabled: true, Host: "jump@jumphost.example.com"}
	if _, err := RunSSHDetailed(cfg, "zinfo", 5); err != nil {
		t.Fatalf("RunSSHDetailed: %v", err)
	}

	joined := strings.Join(recordedArgs(t, argsFile), " ")
	if !strings.Contains(joined, "ProxyCommand=ssh") || !strings.Contains(joined, "-W %h:%p") {
		t.Errorf("ssh was called without a ProxyCommand: %s", joined)
	}
	if !strings.Contains(joined, "-i "+ShellQuote("/keys/zos1")) {
		t.Errorf("the ProxyCommand carries no identity: %s", joined)
	}
}

func TestShellQuoteContainsEmbeddedQuotes(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`plain`, `'plain'`},
		{`two words`, `'two words'`},
		{`it's`, `'it'\''s'`},
		{`; rm -rf /`, `'; rm -rf /'`},
	} {
		if got := ShellQuote(tc.in); got != tc.want {
			t.Errorf("ShellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
