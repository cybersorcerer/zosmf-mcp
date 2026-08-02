package zosmf

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/config"
)

type SSHClientRegistry struct {
	systems map[string]config.SSHSystemConfig
}

func NewSSHClientRegistry(systems []config.SSHSystemConfig) *SSHClientRegistry {
	m := make(map[string]config.SSHSystemConfig, len(systems))
	for _, s := range systems {
		m[strings.ToUpper(s.Name)] = s
	}
	return &SSHClientRegistry{systems: m}
}

func (r *SSHClientRegistry) Get(name string) (config.SSHSystemConfig, error) {
	cfg, ok := r.systems[strings.ToUpper(name)]
	if !ok {
		return config.SSHSystemConfig{}, fmt.Errorf("SSH system not found: %s", name)
	}
	return cfg, nil
}

// sshBaseArgs builds the ssh options shared by RunSSH and RunSSHDetailed.
// The target and the remote command are appended by the caller.
func sshBaseArgs(cfg config.SSHSystemConfig) []string {
	args := []string{
		"-p", fmt.Sprintf("%d", cfg.Port),
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if cfg.KeyFile != "" {
		args = append(args, "-i", cfg.KeyFile)
	}
	if cfg.ProxyJump != nil && cfg.ProxyJump.Active() {
		args = append(args, "-J", cfg.ProxyJump.Host)
	}
	return args
}

func RunSSH(cfg config.SSHSystemConfig, command string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := sshBaseArgs(cfg)

	remoteCmd := command
	if len(cfg.Environment) > 0 {
		var exports []string
		for k, v := range cfg.Environment {
			exports = append(exports, fmt.Sprintf("export %s=%s", k, ShellQuote(v)))
		}
		remoteCmd = strings.Join(exports, " && ") + " && " + command
	}

	args = append(args, fmt.Sprintf("%s@%s", cfg.Username, cfg.Host), remoteCmd)

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

// SSHResult holds the full output of an SSH command execution.
type SSHResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// RunSSHDetailed executes a remote command and returns stdout, stderr and exit code.
func RunSSHDetailed(cfg config.SSHSystemConfig, command string, timeoutSeconds int) (SSHResult, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	args := sshBaseArgs(cfg)

	remoteCmd := command
	if len(cfg.Environment) > 0 {
		var exports []string
		for k, v := range cfg.Environment {
			exports = append(exports, fmt.Sprintf("export %s=%s", k, ShellQuote(v)))
		}
		remoteCmd = strings.Join(exports, " && ") + " && " + command
	}

	args = append(args, fmt.Sprintf("%s@%s", cfg.Username, cfg.Host), remoteCmd)

	cmd := exec.CommandContext(ctx, "ssh", args...)
	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return SSHResult{}, fmt.Errorf("SSH execution failed: %w", err)
		}
	}

	return SSHResult{
		Stdout:   strings.TrimRight(stdoutBuf.String(), "\n"),
		Stderr:   strings.TrimRight(stderrBuf.String(), "\n"),
		ExitCode: exitCode,
	}, nil
}

// ShellQuote wraps s in single quotes, escaping any existing single quotes.
// Use this for user-provided values embedded in SSH commands.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
