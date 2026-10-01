package setup

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
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
// The file it replaces is backed up first.
func WriteEnvFile(path string, vars map[string]string) error {
	existing, order, err := readEnv(path)
	if err != nil {
		return err
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
	if _, err := BackupFile(path); err != nil {
		return err
	}
	return writeFileAtomically(path, []byte(b.String()))
}

// ReadEnvFile returns the variables an environment file sets; a missing
// file sets none.
func ReadEnvFile(path string) (map[string]string, error) {
	vars, _, err := readEnv(path)
	return vars, err
}

func readEnv(path string) (map[string]string, []string, error) {
	return config.ReadEnvFile(path)
}

// writeFileAtomically writes data to path without ever exposing the merged
// content — which may hold secrets — at a looser mode than 0600, even
// briefly. A plain os.WriteFile onto an existing file only applies its mode
// argument when the file is created; an existing file keeps its old mode
// while the new content lands in it, so a 0644 file would hold the secrets,
// world-readable, for the whole write. Instead, a temporary file is created
// at 0600 from its first byte, written, and renamed over path — the rename
// is atomic, so path either still holds its old content or already holds
// the new content at 0600, never something in between, and a crash mid-write
// never truncates the original.
//
// The temporary file is created in the same directory as path: renaming
// across filesystems is not atomic, and os.TempDir() is often a different
// one than path's directory.
func writeFileAtomically(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".env-*")
	if err != nil {
		return fmt.Errorf("creating a temporary file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	// os.CreateTemp already makes the file 0600 on Unix, but that is a
	// platform note, not a guarantee — make it explicit in the code.
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("setting permissions on %s: %w", tmpPath, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	renamed = true
	return nil
}

// GenerateAPIKey produces a bearer token of the same shape as `make apikey`.
func GenerateAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating an API key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}
