package zosmf

import (
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
)

func hasFlagValue(args []string, flag, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

// proxyCommand returns the value of the -o ProxyCommand=... option.
func proxyCommand(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-o" {
			if cmd, ok := strings.CutPrefix(args[i+1], "ProxyCommand="); ok {
				return cmd
			}
		}
	}
	return ""
}

// The jump hop is a child ssh that inherits nothing from the parent command
// line, so the key and BatchMode have to be repeated inside ProxyCommand.
func TestSSHBaseArgsIncludesJumpWhenActive(t *testing.T) {
	args := sshBaseArgs(config.SSHSystemConfig{
		Name:      "SYS1",
		Host:      "zos1.example.com",
		Port:      22,
		Username:  "ibmuser",
		KeyFile:   "/keys/zos1",
		ProxyJump: &config.SSHProxyJumpConfig{Enabled: true, Host: "jump@jumphost.example.com"},
	})
	cmd := proxyCommand(args)
	if cmd == "" {
		t.Fatalf("expected a -o ProxyCommand=... in %v", args)
	}
	for _, want := range []string{
		"-W %h:%p",
		ShellQuote("jump@jumphost.example.com"),
		"-i " + ShellQuote("/keys/zos1"),
		"BatchMode=yes",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("expected %q in ProxyCommand %q", want, cmd)
		}
	}
}

func TestSSHBaseArgsJumpWithoutKeyFile(t *testing.T) {
	args := sshBaseArgs(config.SSHSystemConfig{
		Name: "SYS1", Host: "zos1.example.com", Port: 22, Username: "ibmuser",
		ProxyJump: &config.SSHProxyJumpConfig{Enabled: true, Host: "jump@jumphost.example.com"},
	})
	cmd := proxyCommand(args)
	if cmd == "" {
		t.Fatalf("expected a -o ProxyCommand=... in %v", args)
	}
	if strings.Contains(cmd, "-i ") {
		t.Errorf("expected no -i in ProxyCommand %q", cmd)
	}
}

func TestSSHBaseArgsOmitsJumpWhenInactive(t *testing.T) {
	for _, tc := range []struct {
		name string
		jump *config.SSHProxyJumpConfig
	}{
		{"absent", nil},
		{"disabled", &config.SSHProxyJumpConfig{Enabled: false, Host: "jump@jumphost.example.com"}},
		{"no host", &config.SSHProxyJumpConfig{Enabled: true, Host: ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := sshBaseArgs(config.SSHSystemConfig{
				Name: "SYS1", Host: "zos1.example.com", Port: 22,
				Username: "ibmuser", ProxyJump: tc.jump,
			})
			if proxyCommand(args) != "" {
				t.Errorf("expected no ProxyCommand in %v", args)
			}
		})
	}
}

func TestSSHBaseArgsKeepsExistingOptions(t *testing.T) {
	args := sshBaseArgs(config.SSHSystemConfig{
		Name: "SYS1", Host: "zos1.example.com", Port: 2222,
		Username: "ibmuser", KeyFile: "/keys/zos1",
	})
	if !hasFlagValue(args, "-p", "2222") {
		t.Errorf("expected -p 2222 in %v", args)
	}
	if !hasFlagValue(args, "-i", "/keys/zos1") {
		t.Errorf("expected -i /keys/zos1 in %v", args)
	}
	if !hasFlagValue(args, "-o", "BatchMode=yes") {
		t.Errorf("expected -o BatchMode=yes in %v", args)
	}
	if !hasFlagValue(args, "-o", "StrictHostKeyChecking=accept-new") {
		t.Errorf("expected -o StrictHostKeyChecking=accept-new in %v", args)
	}
}
