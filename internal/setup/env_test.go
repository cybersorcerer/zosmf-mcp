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

func TestWriteEnvFileLeavesNoTemporaryFileBehindAfterSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("EXISTING=keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteEnvFile(path, map[string]string{"ZOSMF_MAINT_PASSWORD": "new"}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != filepath.Base(path) && e.Name() != BackupDirName {
			t.Errorf("temporary file left behind: %q in %s", e.Name(), dir)
		}
	}
}

func TestWriteEnvFileLeavesTheOriginalUnchangedWhenTheWriteFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bits this test relies on")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	original := []byte("EXISTING=keep\nZOSMF_MAINT_PASSWORD=old\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	// Without write permission on the directory, the temporary file cannot
	// be created, so the write must fail before it ever touches path.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
	})

	if err := WriteEnvFile(path, map[string]string{"ZOSMF_MAINT_PASSWORD": "new"}); err == nil {
		t.Fatal("expected an error when the directory is not writable")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Errorf("the original file was changed on a failed write:\n%s", data)
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

func TestWriteEnvFileBacksUpTheReplacedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("ZOSMF_MAINT_PASSWORD=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteEnvFile(path, map[string]string{"ZOSMF_MAINT_PASSWORD": "new"}); err != nil {
		t.Fatal(err)
	}
	list, err := ListBackups(path)
	if err != nil || len(list) != 1 {
		t.Fatalf("backups = %v, %v; want one", list, err)
	}
	if got, _ := os.ReadFile(list[0].Path); string(got) != "ZOSMF_MAINT_PASSWORD=old\n" {
		t.Error("the backup does not hold the replaced file")
	}
}
