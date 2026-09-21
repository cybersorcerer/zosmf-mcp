package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
)

func run(t *testing.T, tty bool, args ...string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := runInit(args, &out, &errBuf, tty)
	return code, out.String(), errBuf.String()
}

func TestInitConfigWritesASystem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.yaml")

	code, _, errOut := run(t, false, "config", "--file", path,
		"--name", "MAINT", "--host", "https://zos.example.com:443",
		"--user", "IBMUSER", "--csi", "A.CSI,B.CSI", "--zones", "MVST100")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"name: MAINT", "A.CSI", "B.CSI", "MVST100"} {
		if !strings.Contains(text, want) {
			t.Errorf("file lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "password") {
		t.Errorf("a password reference was written:\n%s", text)
	}
}

func TestInitConfigWithoutTTYAndWithoutFlagsExplainsItself(t *testing.T) {
	code, _, errOut := run(t, false, "config", "--file", filepath.Join(t.TempDir(), "c.yaml"))
	if code == 0 {
		t.Fatal("expected a non-zero exit")
	}
	for _, want := range []string{"--questions", "--name"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr %q does not mention %q", errOut, want)
		}
	}
}

func TestInitConfigQuestionsIsJSON(t *testing.T) {
	code, out, _ := run(t, false, "config", "--questions")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var doc struct {
		Command string `json:"command"`
		Fields  []any  `json:"fields"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if doc.Command != "init config" || len(doc.Fields) == 0 {
		t.Errorf("unexpected catalog: %+v", doc)
	}
}

func TestInitConfigPrintWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.yaml")
	code, out, _ := run(t, false, "config", "--file", path, "--print",
		"--name", "MAINT", "--host", "https://zos.example.com:443", "--user", "IBMUSER")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "name: MAINT") {
		t.Errorf("stdout carries no rendered config:\n%s", out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("--print created the file")
	}
}

func TestInitConfigRejectsANonNumericPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.yaml")

	code, _, errOut := run(t, false, "config", "--file", path,
		"--name", "MAINT", "--host", "https://zos.example.com:443",
		"--user", "IBMUSER", "--port", "abc")
	if code == 0 {
		t.Fatalf("expected a non-zero exit, stderr: %s", errOut)
	}
	if !strings.Contains(errOut, "--port") {
		t.Errorf("stderr %q does not name the bad flag", errOut)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("a non-numeric port should not have written the file")
	}
}

func TestInitConfigCreatesTheParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "conf.yaml")

	code, _, errOut := run(t, false, "config", "--file", path,
		"--name", "MAINT", "--host", "https://zos.example.com:443", "--user", "IBMUSER")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected the file to have been created: %v", err)
	}
}

func TestInitConfigWritesSSHSystemWhenZOAUPathGiven(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.yaml")
	code, _, errOut := run(t, false, "config", "--file", path,
		"--name", "MAINT", "--host", "https://zos.example.com:443", "--user", "IBMUSER",
		"--ssh-host", "zos.example.com", "--ssh-user", "ibmuser",
		"--ssh-key-file", "/keys/id_rsa", "--zoau-path", "/usr/lpp/IBM/zoautil")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	for _, want := range []string{"ssh_systems:", "key_file: /keys/id_rsa", "ZOAU_HOME:", "LIBPATH:"} {
		if !strings.Contains(text, want) {
			t.Errorf("file lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "zoau-path:") {
		t.Errorf("the helper answer leaked into the file:\n%s", text)
	}
}

func TestInitConfigRejectsZOAUPathWithoutTheRestOfTheGroup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.yaml")
	code, _, errOut := run(t, false, "config", "--file", path,
		"--name", "MAINT", "--host", "https://zos.example.com:443", "--user", "IBMUSER",
		"--zoau-path", "/usr/lpp/IBM/zoautil")
	if code == 0 {
		t.Fatalf("expected a non-zero exit, stderr: %s", errOut)
	}
	for _, want := range []string{"--ssh-host", "--ssh-user", "--ssh-key-file"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr %q does not name missing flag %q", errOut, want)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("an incomplete ZOAU invocation should not have written the file")
	}
}

func TestInitConfigWithoutZOAUFlagsWritesNoSSHSystems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.yaml")
	code, _, errOut := run(t, false, "config", "--file", path,
		"--name", "MAINT", "--host", "https://zos.example.com:443", "--user", "IBMUSER")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "ssh_systems:") {
		t.Errorf("no ZOAU flags were given but ssh_systems was written:\n%s", data)
	}
}

func TestInitConfigCheckRejectsABrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.yaml")
	if err := os.WriteFile(path, []byte("servers:\n  - name: SYS1\n    host: https://z.example.com\n    auth:\n      cert_file: /x.pem\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := run(t, false, "config", "--file", path, "--check")
	if code == 0 {
		t.Fatalf("expected a non-zero exit, stderr: %s", errOut)
	}
	if !strings.Contains(errOut, "auth type missing") {
		t.Errorf("stderr should carry the loader's complaint, got: %s", errOut)
	}
}

// A file written by init config alone must be able to start the server —
// without this, the guided setup ends on a file main.go refuses to run.
func TestInitConfigWritesADefaultTransportBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.yaml")
	code, _, errOut := run(t, false, "config", "--file", path,
		"--name", "MAINT", "--host", "https://zos.example.com:443", "--user", "IBMUSER")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load rejected the written file: %v", err)
	}
	if !cfg.Transport.Stdio {
		t.Error("a file written by init config alone must have stdio transport enabled")
	}
}
