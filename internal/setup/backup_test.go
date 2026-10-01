package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func withClock(t *testing.T, start time.Time) func() {
	t.Helper()
	at := start
	old := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = old })
	return func() { at = at.Add(time.Second) }
}

func TestBackupFileCopiesIntoTheBackupFolder(t *testing.T) {
	withClock(t, time.Date(2026, 10, 1, 14, 5, 30, 0, time.Local))
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := BackupFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(path), BackupDirName, ".env.20261001-140530")
	if b.Path != want {
		t.Errorf("backup at %s, want %s", b.Path, want)
	}
	info, err := os.Stat(want)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode %v, want 600", info.Mode().Perm())
	}
}

func TestBackupFileOfAMissingFileIsNoBackup(t *testing.T) {
	b, err := BackupFile(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil || b != nil {
		t.Errorf("got %v, %v; want nil, nil", b, err)
	}
}

// Backing up content identical to the newest backup must not push a real
// older state out of the ten kept.
func TestBackupFileSkipsAnUnchangedFile(t *testing.T) {
	tick := withClock(t, time.Date(2026, 10, 1, 14, 0, 0, 0, time.Local))
	path := filepath.Join(t.TempDir(), "c.yaml")
	_ = os.WriteFile(path, []byte("a: 1\n"), 0o600)
	_, _ = BackupFile(path)
	tick()
	_, _ = BackupFile(path)
	list, _ := ListBackups(path)
	if len(list) != 1 {
		t.Errorf("%d backups, want 1", len(list))
	}
}

func TestBackupFileKeepsTheNewestTen(t *testing.T) {
	tick := withClock(t, time.Date(2026, 10, 1, 14, 0, 0, 0, time.Local))
	path := filepath.Join(t.TempDir(), "c.yaml")
	for i := range 12 {
		_ = os.WriteFile(path, fmt.Appendf(nil, "a: %d\n", i), 0o600)
		if _, err := BackupFile(path); err != nil {
			t.Fatal(err)
		}
		tick()
	}
	list, _ := ListBackups(path)
	if len(list) != 10 {
		t.Fatalf("%d backups, want 10", len(list))
	}
	if list[0].Stamp != "20261001-140011" || list[9].Stamp != "20261001-140002" {
		t.Errorf("kept %s … %s, want the newest ten, newest first", list[0].Stamp, list[9].Stamp)
	}
}

// Two different states saved within one second must both survive.
func TestBackupFileWithinTheSameSecond(t *testing.T) {
	withClock(t, time.Date(2026, 10, 1, 14, 0, 0, 0, time.Local))
	path := filepath.Join(t.TempDir(), "c.yaml")
	_ = os.WriteFile(path, []byte("a: 1\n"), 0o600)
	_, _ = BackupFile(path)
	_ = os.WriteFile(path, []byte("a: 2\n"), 0o600)
	_, _ = BackupFile(path)
	if list, _ := ListBackups(path); len(list) != 2 {
		t.Errorf("%d backups, want 2", len(list))
	}
}

func TestRestoreBackupIsReversible(t *testing.T) {
	tick := withClock(t, time.Date(2026, 10, 1, 14, 0, 0, 0, time.Local))
	path := filepath.Join(t.TempDir(), "c.yaml")
	_ = os.WriteFile(path, []byte("old\n"), 0o600)
	old, _ := BackupFile(path)
	_ = os.WriteFile(path, []byte("new\n"), 0o600)
	tick()

	saved, err := RestoreBackup(path, old.Stamp)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "old\n" {
		t.Errorf("file holds %q after restore, want the backup", got)
	}
	if saved == nil {
		t.Fatal("the state before the restore was not backed up")
	}
	if got, _ := os.ReadFile(saved.Path); string(got) != "new\n" {
		t.Errorf("pre-restore backup holds %q, want the replaced state", got)
	}
}

func TestRestoreBackupUnknownStamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	_ = os.WriteFile(path, []byte("x\n"), 0o600)
	if _, err := RestoreBackup(path, "20000101-000000"); err == nil {
		t.Error("restoring an unknown backup must fail")
	}
}
