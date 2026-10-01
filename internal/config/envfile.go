package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// EnvFilePath is where the secrets for the configuration at configPath live:
// `zosmf-mcp init env` writes them there, and Load reads them from there.
func EnvFilePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), ".env")
}

// ReadEnvFile parses KEY=value lines, in file order. Blank lines, comments
// and an `export ` prefix are skipped, and one pair of quotes around a value
// is removed, as a shell sourcing the file would. A missing file sets nothing.
func ReadEnvFile(path string) (map[string]string, []string, error) {
	vars := map[string]string{}
	var order []string
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return vars, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

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
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		if _, known := vars[key]; !known {
			order = append(order, key)
		}
		vars[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return vars, order, nil
}

// loadEnvFile reads the .env beside the configuration. One that others can
// read holds passwords nobody else should have; it is ignored, with a
// warning, rather than trusted.
func (c *Config) loadEnvFile(configPath string) error {
	path := EnvFilePath(configPath)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		c.Warnings = append(c.Warnings, fmt.Sprintf(
			"ignored %s: mode %o lets others read it — run `chmod 600 %s`", path, info.Mode().Perm(), path))
		return nil
	}
	vars, _, err := ReadEnvFile(path)
	if err != nil {
		return err
	}
	c.EnvFile = path
	c.fileEnv = vars
	return nil
}

// getenv looks a variable up in the process environment first, then in the
// .env file, and says whether the file supplied it.
func (c *Config) getenv(name string) (value string, fromFile bool) {
	if v := os.Getenv(name); v != "" {
		return v, false
	}
	if v := c.fileEnv[name]; v != "" {
		return v, true
	}
	return "", false
}
