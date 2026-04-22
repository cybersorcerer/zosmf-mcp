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

// ShellQuote wraps s in single quotes, escaping any existing single quotes.
// Use this for user-provided values embedded in SSH commands.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
