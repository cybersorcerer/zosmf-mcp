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

// Without --env-file the secrets go next to the configuration, not into
// whatever directory the command happens to run in — a repository, say.
func TestInitEnvDefaultsToTheConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "conf.yaml")
	if err := os.WriteFile(conf, []byte("servers:\n  - name: MAINT\n    host: https://zos.example.com:443\n    user: IBMUSER\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	t.Setenv("ZOSMF_MAINT_PASSWORD", "pw")

	code, out, errOut := run(t, false, "env", "--file", conf)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	want := filepath.Join(dir, ".env")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("no %s: %v", want, err)
	}
	if _, err := os.Stat(".env"); !os.IsNotExist(err) {
		t.Error(".env was written into the working directory")
	}
	if !strings.Contains(out, want) {
		t.Errorf("output does not name the full path:\n%s", out)
	}
}

// twoSystems writes a config with MAIN and TEST and a .env next to it that
// already holds TEST's password.
func twoSystems(t *testing.T) (conf, env string) {
	t.Helper()
	dir := t.TempDir()
	conf = filepath.Join(dir, "conf.yaml")
	env = filepath.Join(dir, ".env")
	_ = os.WriteFile(conf, []byte("servers:\n  - name: MAIN\n    host: https://zos:443\n    user: U\n  - name: TEST\n    host: https://zos2:443\n    user: U\n"), 0o600)
	_ = os.WriteFile(env, []byte("ZOSMF_TEST_PASSWORD=kept\n"), 0o600)
	t.Setenv("ZOSMF_MAIN_PASSWORD", "")
	t.Setenv("ZOSMF_TEST_PASSWORD", "")
	return conf, env
}

// Adding a second system must not ask again for the first one's password.
func TestInitEnvQuestionsSkipsVariablesTheFileHas(t *testing.T) {
	conf, _ := twoSystems(t)
	code, out, errOut := run(t, false, "env", "--file", conf, "--questions")
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, errOut)
	}
	if !strings.Contains(out, "ZOSMF_MAIN_PASSWORD") || strings.Contains(out, "ZOSMF_TEST_PASSWORD") {
		t.Errorf("want only the missing MAIN password:\n%s", out)
	}
}

func TestInitEnvKeepsWhatTheFileHas(t *testing.T) {
	conf, env := twoSystems(t)
	t.Setenv("ZOSMF_MAIN_PASSWORD", "new")
	code, out, errOut := run(t, false, "env", "--file", conf)
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, errOut)
	}
	if got := envValue(t, env, "ZOSMF_TEST_PASSWORD"); got != "kept" {
		t.Errorf("TEST password = %q, want the one already in the file", got)
	}
	if got := envValue(t, env, "ZOSMF_MAIN_PASSWORD"); got != "new" {
		t.Errorf("MAIN password = %q, want the exported one", got)
	}
	if !strings.Contains(out, "ZOSMF_TEST_PASSWORD") {
		t.Errorf("output does not say which variable was kept:\n%s", out)
	}
}

func TestInitEnvWithNothingMissing(t *testing.T) {
	conf, env := twoSystems(t)
	_ = os.WriteFile(env, []byte("ZOSMF_TEST_PASSWORD=kept\nZOSMF_MAIN_PASSWORD=also\n"), 0o600)

	code, out, _ := run(t, false, "env", "--file", conf, "--questions")
	if code != 0 || !strings.Contains(out, `"fields": []`) {
		t.Errorf("exit %d; want an empty field list:\n%s", code, out)
	}
	code, out, errOut := run(t, false, "env", "--file", conf)
	if code != 0 || !strings.Contains(out, "nothing to do") {
		t.Errorf("exit %d, out %q, stderr %q; want success saying nothing to do", code, out, errOut)
	}
}

// After init env, --check proves the password reaches the server without
// sourcing anything: the server reads the .env beside the configuration.
func TestInitEnvThenCheckFindsThePassword(t *testing.T) {
	conf, _ := twoSystems(t)
	t.Setenv("ZOSMF_MAIN_PASSWORD", "pw")
	if code, _, errOut := run(t, false, "env", "--file", conf); code != 0 {
		t.Fatalf("init env: exit %d, stderr %s", code, errOut)
	}
	_ = os.Unsetenv("ZOSMF_MAIN_PASSWORD")
	code, out, errOut := run(t, false, "config", "--file", conf, "--check")
	if code != 0 {
		t.Fatalf("--check: exit %d, stderr %s", code, errOut)
	}
	for _, want := range []string{"system MAIN: password from env-file", "system TEST: password from env-file"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}
