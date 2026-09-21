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

func TestInitEnvWritesAKeyWithoutPrintingIt(t *testing.T) {
	// Without a password already in the environment, runInitEnv would stop
	// before it ever reaches the write path — the config below needs
	// ZOSMF_MAINT_PASSWORD, and this test runs with no TTY to ask on. Set it
	// so the test actually exercises WriteEnvFile and the key generation it
	// means to check.
	t.Setenv("ZOSMF_MAINT_PASSWORD", "x")

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

	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "MCP_API_KEY=") {
		t.Errorf(".env has no API key:\n%s", data)
	}
	key := strings.TrimPrefix(strings.TrimSpace(string(data)), "MCP_API_KEY=")
	if key != "" && strings.Contains(out, key) {
		t.Error("the generated key was printed to stdout")
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
