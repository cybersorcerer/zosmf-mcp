package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteEnvFileMergesAndKeepsMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("EXISTING=keep\nZOSMF_MAINT_PASSWORD=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteEnvFile(path, map[string]string{"ZOSMF_MAINT_PASSWORD": "new"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "EXISTING=keep") {
		t.Errorf("an unrelated variable was dropped:\n%s", text)
	}
	if strings.Contains(text, "=old") || !strings.Contains(text, "ZOSMF_MAINT_PASSWORD=new") {
		t.Errorf("the value was not replaced:\n%s", text)
	}
	if strings.Contains(text, "export ") {
		t.Errorf("export form is rejected by --env-file:\n%s", text)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 600", info.Mode().Perm())
	}
}

func TestWriteEnvFileTightensPermissionsOnAnExistingFile(t *testing.T) {
	// os.WriteFile only applies its mode argument when it creates the file;
	// an existing file, e.g. one a user created by hand with `touch` (0644),
	// keeps its old mode otherwise. This file holds secrets.
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("EXISTING=keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteEnvFile(path, map[string]string{"ZOSMF_MAINT_PASSWORD": "new"}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 600 even though the file pre-existed with a looser mode", info.Mode().Perm())
	}
}

func TestGenerateAPIKeyIsLongAndUnique(t *testing.T) {
	a, err := GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := GenerateAPIKey()
	if len(a) < 32 {
		t.Errorf("key is %d characters, want at least 32", len(a))
	}
	if a == b {
		t.Error("two calls produced the same key")
	}
}
