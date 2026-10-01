package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/setup"
)

// restoreFixture leaves one backup of the config and one of .env behind,
// and returns the stamp of the config backup.
func restoreFixture(t *testing.T) (conf, env, stamp string) {
	t.Helper()
	dir := t.TempDir()
	conf = filepath.Join(dir, "conf.yaml")
	env = filepath.Join(dir, ".env")
	_ = os.WriteFile(conf, []byte("old: config\n"), 0o600)
	_ = os.WriteFile(env, []byte("A=old\n"), 0o600)
	b, err := setup.BackupFile(conf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := setup.BackupFile(env); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(conf, []byte("new: config\n"), 0o600)
	_ = os.WriteFile(env, []byte("A=new\n"), 0o600)
	return conf, env, b.Stamp
}

func TestInitRestoreListIsJSON(t *testing.T) {
	conf, env, stamp := restoreFixture(t)
	code, out, errOut := run(t, false, "restore", "--file", conf, "--env-file", env, "--list")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	var got struct {
		Backups []struct {
			Target    string `json:"target"`
			Timestamp string `json:"timestamp"`
		} `json:"backups"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(got.Backups) != 2 {
		t.Fatalf("%d backups listed, want 2:\n%s", len(got.Backups), out)
	}
	targets := got.Backups[0].Target + "," + got.Backups[1].Target
	if targets != "config,env" || got.Backups[0].Timestamp != stamp {
		t.Errorf("listing = %+v", got.Backups)
	}
}

func TestInitRestoreBringsBackTheConfig(t *testing.T) {
	conf, env, stamp := restoreFixture(t)
	code, out, errOut := run(t, false, "restore", "--file", conf, "--env-file", env, "--backup", stamp, "--target", "config")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if got, _ := os.ReadFile(conf); string(got) != "old: config\n" {
		t.Errorf("config = %q after restore", got)
	}
	if got, _ := os.ReadFile(env); string(got) != "A=new\n" {
		t.Error(".env was touched by a config restore")
	}
	if !strings.Contains(out, setup.BackupDirName) {
		t.Errorf("output does not say where the replaced version went:\n%s", out)
	}
}

// Config and .env backed up in the same second share a stamp; restoring
// both by accident would be worse than asking.
func TestInitRestoreAmbiguousStampNeedsATarget(t *testing.T) {
	conf, env, stamp := restoreFixture(t)
	envList, _ := setup.ListBackups(env)
	if envList[0].Stamp != stamp {
		t.Skip("the fixture's two backups fell into different seconds")
	}
	code, _, errOut := run(t, false, "restore", "--file", conf, "--env-file", env, "--backup", stamp)
	if code != 2 || !strings.Contains(errOut, "--target") {
		t.Errorf("exit %d, stderr %q; want 2 and a hint at --target", code, errOut)
	}
}

func TestInitRestoreUnknownStamp(t *testing.T) {
	conf, env, _ := restoreFixture(t)
	code, _, _ := run(t, false, "restore", "--file", conf, "--env-file", env, "--backup", "20000101-000000")
	if code == 0 {
		t.Error("an unknown backup must fail")
	}
}
