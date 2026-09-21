package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
