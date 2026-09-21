package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitEnvQuestionsListsTheSystemVariables(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "conf.yaml")
	if err := os.WriteFile(conf, []byte(`
servers:
  - name: MAINT
    host: https://zos.example.com:443
    user: IBMUSER
  - name: PROD
    host: https://zos2.example.com:443
    user: IBMUSER
`), 0o600); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := run(t, false, "env", "--file", conf, "--questions")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	for _, want := range []string{"ZOSMF_MAINT_PASSWORD", "ZOSMF_PROD_PASSWORD"} {
		if !strings.Contains(out, want) {
			t.Errorf("catalog lacks %q:\n%s", want, out)
		}
	}
}

// envValue reads one KEY=value line from a written env file, the same way
// WriteEnvFile parses one back in. Cutting the *whole file* on a prefix, as
// this test used to, folds the rest of the file — every other line — into
// what it then calls "the value", so a later substring check against that
// string can never realistically match anything on stdout: the check was
// unfalsifiable, not passing.
func envValue(t *testing.T, path, key string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, found := strings.Cut(line, "=")
		if found && k == key {
			return v
		}
	}
	return ""
}

func TestInitEnvWritesAKeyWithoutPrintingIt(t *testing.T) {
	// Without a password already in the environment, runInitEnv would stop
	// before it ever reaches the write path — the config below needs
	// ZOSMF_MAINT_PASSWORD, and this test runs with no TTY to ask on. Set it
	// so the test actually exercises WriteEnvFile and the key generation it
	// means to check. The value is distinctive, not "x": a one-character
	// sentinel is too likely to appear incidentally (in the generated key,
	// in a path, anywhere) for its absence to mean anything.
	const password = "S3cr3t-Watermelon-77Q"
	t.Setenv("ZOSMF_MAINT_PASSWORD", password)

	dir := t.TempDir()
	conf := filepath.Join(dir, "conf.yaml")
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(conf, []byte(`
servers:
  - name: MAINT
    host: https://zos.example.com:443
    user: IBMUSER
`), 0o600); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := run(t, false, "env", "--file", conf, "--env-file", envFile, "--generate-api-key")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}

	key := envValue(t, envFile, "MCP_API_KEY")
	if key == "" {
		data, _ := os.ReadFile(envFile)
		t.Fatalf(".env has no MCP_API_KEY:\n%s", data)
	}
	if got := envValue(t, envFile, "ZOSMF_MAINT_PASSWORD"); got != password {
		t.Fatalf(".env carries %q for ZOSMF_MAINT_PASSWORD, want %q", got, password)
	}

	// Neither secret may appear anywhere on either stream.
	for _, secret := range []string{key, password} {
		if strings.Contains(out, secret) {
			t.Errorf("a secret was printed to stdout:\n%s", out)
		}
		if strings.Contains(errOut, secret) {
			t.Errorf("a secret was printed to stderr:\n%s", errOut)
		}
	}

	info, err := os.Stat(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 600", info.Mode().Perm())
	}
}

func TestInitEnvWithoutTTYAndMissingPasswordExplainsItself(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "conf.yaml")
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(conf, []byte(`
servers:
  - name: MAINT
    host: https://zos.example.com:443
    user: IBMUSER
`), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := run(t, false, "env", "--file", conf, "--env-file", envFile)
	if code == 0 {
		t.Fatal("expected a non-zero exit")
	}
	if !strings.Contains(errOut, "ZOSMF_MAINT_PASSWORD") {
		t.Errorf("stderr %q does not name the missing variable", errOut)
	}
	if _, err := os.Stat(envFile); !os.IsNotExist(err) {
		t.Error("no env file should have been written")
	}
}
