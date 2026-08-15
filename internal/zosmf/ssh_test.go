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

func TestSSHBaseArgsIncludesJumpWhenActive(t *testing.T) {
	args := sshBaseArgs(config.SSHSystemConfig{
		Name:      "SYS1",
		Host:      "zos1.example.com",
		Port:      22,
		Username:  "ibmuser",
		ProxyJump: &config.SSHProxyJumpConfig{Enabled: true, Host: "jump@jumphost.example.com"},
	})
	if !hasFlagValue(args, "-J", "jump@jumphost.example.com") {
		t.Errorf("expected -J jump@jumphost.example.com in %v", args)
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
			if strings.Contains(strings.Join(args, " "), "-J") {
				t.Errorf("expected no -J in %v", args)
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
