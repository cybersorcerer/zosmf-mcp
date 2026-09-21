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

// A flag the user does not pass on a rerun must not reset the value that
// invocation already wrote — the bug this guards was BindFlags handing
// every unset flag the catalog default, so a rerun that only added --csi
// silently flipped rejectUnauthorized back to true on a self-signed test
// LPAR, in a file three other tools also read.
func TestInitConfigRerunWithoutAFlagKeepsTheExistingValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.yaml")
	code, _, errOut := run(t, false, "config", "--file", path,
		"--name", "MAINT", "--host", "https://zos.example.com:443", "--user", "IBMUSER",
		"--reject-unauthorized", "false")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}

	code, _, errOut = run(t, false, "config", "--file", path,
		"--name", "MAINT", "--host", "https://zos.example.com:443", "--user", "IBMUSER",
		"--csi", "A.CSI")
	if code != 0 {
		t.Fatalf("rerun: exit %d, stderr: %s", code, errOut)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "rejectUnauthorized: false") {
		t.Errorf("rerun without --reject-unauthorized reset it to the catalog default:\n%s", data)
	}
}

// Same shape, the ZOAU group's own field with a catalog default: a rerun
// that re-engages the group without repeating --ssh-port must not reset a
// configured non-default port.
func TestInitConfigRerunWithoutSSHPortKeepsTheExistingValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.yaml")
	code, _, errOut := run(t, false, "config", "--file", path,
		"--name", "MAINT", "--host", "https://zos.example.com:443", "--user", "IBMUSER",
		"--ssh-host", "zos.example.com", "--ssh-port", "2222", "--ssh-user", "ibmuser",
		"--ssh-key-file", "/keys/id_rsa", "--zoau-path", "/usr/lpp/IBM/zoautil")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}

	code, _, errOut = run(t, false, "config", "--file", path,
		"--name", "MAINT", "--host", "https://zos.example.com:443", "--user", "IBMUSER",
		"--ssh-host", "zos.example.com", "--ssh-user", "ibmuser",
		"--ssh-key-file", "/keys/id_rsa", "--zoau-path", "/usr/lpp/IBM/zoautil")
	if code != 0 {
		t.Fatalf("rerun: exit %d, stderr: %s", code, errOut)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "port: 2222") {
		t.Errorf("rerun without --ssh-port reset it to the catalog default 22:\n%s", data)
	}
}

// An agent following agents/setup.md step 1 reads --questions to learn
// what it can ask for. The ZOAU group has to be in there, or it can never
// be configured non-interactively by anything that only reads this list.
func TestInitConfigQuestionsIncludesTheZOAUGroup(t *testing.T) {
	code, out, errOut := run(t, false, "config", "--questions")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	var doc struct {
		Fields []struct {
			ID    string `json:"id"`
			Group string `json:"group"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	zoau := 0
	for _, f := range doc.Fields {
		if f.Group == "zoau" {
			zoau++
		}
	}
	if zoau == 0 {
		t.Errorf("--questions carries no zoau group fields:\n%s", out)
	}
}
