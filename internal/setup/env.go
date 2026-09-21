package setup

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
)

// RequiredEnvVars lists the variables this configuration needs: one password
// per system, by convention, plus the API key when an HTTP transport is on.
// Variables the file already references explicitly are included as well.
func RequiredEnvVars(cfg *config.Config, raw []byte) []string {
	seen := map[string]bool{}
	var vars []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			vars = append(vars, name)
		}
	}
	for _, sys := range cfg.Systems {
		add(config.PasswordEnvVar(sys.Name))
	}
	for _, name := range referencedVars(string(raw)) {
		add(name)
	}
	if cfg.Transport.HTTP.Enabled || cfg.Transport.StreamableHTTP.Enabled {
		add("MCP_API_KEY")
	}
	return vars
}

// referencedVars finds ${NAME} references so a hand-written file keeps
// working alongside the convention.
func referencedVars(text string) []string {
	var out []string
	for {
		start := strings.Index(text, "${")
		if start < 0 {
			return out
		}
		end := strings.Index(text[start:], "}")
		if end < 0 {
			return out
		}
		out = append(out, text[start+2:start+end])
		text = text[start+end:]
	}
}

// WriteEnvFile merges the given variables into an existing file, keeping
// everything else, and writes it back with mode 600 in plain KEY=value form.
func WriteEnvFile(path string, vars map[string]string) error {
	existing := map[string]string{}
	var order []string

	if f, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			line = strings.TrimPrefix(line, "export ")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, found := strings.Cut(line, "=")
			if !found {
				continue
			}
			if _, known := existing[key]; !known {
				order = append(order, key)
			}
			existing[key] = value
		}
		_ = f.Close()
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	var added []string
	for key := range vars {
		if _, known := existing[key]; !known {
			added = append(added, key)
		}
		existing[key] = vars[key]
	}
	sort.Strings(added)
	order = append(order, added...)

	var b strings.Builder
	for _, key := range order {
		_, _ = fmt.Fprintf(&b, "%s=%s\n", key, existing[key])
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return err
	}
	// os.WriteFile only applies the given mode when it creates the file; an
	// existing file (hand-created with looser permissions, say) keeps its
	// old mode otherwise. This file holds secrets, so tighten it explicitly.
	return os.Chmod(path, 0o600)
}

// GenerateAPIKey produces a bearer token of the same shape as `make apikey`.
func GenerateAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating an API key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}
