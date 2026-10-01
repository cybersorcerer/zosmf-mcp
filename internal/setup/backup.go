package setup

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BackupDirName is the folder, next to the file it protects, that holds the
// timestamped copies made before every write and every restore.
const BackupDirName = ".zosmf-mcp-backups"

const keepBackups = 10

const stampLayout = "20060102-150405"

// now is replaced by the tests.
var now = time.Now

// Backup is one saved copy of File.
type Backup struct {
	File  string `json:"file"`
	Stamp string `json:"timestamp"`
	Path  string `json:"path"`
}

func backupDir(path string) string {
	return filepath.Join(filepath.Dir(path), BackupDirName)
}

// BackupFile copies path into its backup folder at mode 600 — the file may
// be .env — and keeps the newest ten. A missing file, or one identical to
// its newest backup, needs no backup and returns nil.
func BackupFile(path string) (*Backup, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s for its backup: %w", path, err)
	}
	list, err := ListBackups(path)
	if err != nil {
		return nil, err
	}
	if len(list) > 0 {
		if newest, err := os.ReadFile(list[0].Path); err == nil && bytes.Equal(newest, data) {
			return nil, nil
		}
	}

	dir := backupDir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}
	stamp := now().Format(stampLayout)
	base := stamp
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, filepath.Base(path)+"."+stamp)); os.IsNotExist(err) {
			break
		}
		stamp = fmt.Sprintf("%s-%d", base, i)
	}
	b := &Backup{File: path, Stamp: stamp, Path: filepath.Join(dir, filepath.Base(path)+"."+stamp)}
	if err := writeFileAtomically(b.Path, data); err != nil {
		return nil, fmt.Errorf("writing the backup: %w", err)
	}

	list, err = ListBackups(path)
	if err != nil {
		return nil, err
	}
	for _, old := range list[min(len(list), keepBackups):] {
		_ = os.Remove(old.Path)
	}
	return b, nil
}

// ListBackups returns the backups of path, newest first.
func ListBackups(path string) ([]Backup, error) {
	entries, err := os.ReadDir(backupDir(path))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	prefix := filepath.Base(path) + "."
	var list []Backup
	for _, e := range entries {
		stamp, ok := strings.CutPrefix(e.Name(), prefix)
		if !ok || e.IsDir() || !isStamp(stamp) {
			continue
		}
		list = append(list, Backup{File: path, Stamp: stamp, Path: filepath.Join(backupDir(path), e.Name())})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Stamp > list[j].Stamp })
	return list, nil
}

// isStamp keeps .env's backups out of .env.local's listing and the like.
// A stamp is the time, plus "-N" when several backups share a second.
func isStamp(s string) bool {
	if len(s) < len(stampLayout) {
		return false
	}
	if _, err := time.Parse(stampLayout, s[:len(stampLayout)]); err != nil {
		return false
	}
	rest := s[len(stampLayout):]
	if rest == "" {
		return true
	}
	n, ok := strings.CutPrefix(rest, "-")
	_, err := strconv.Atoi(n)
	return ok && err == nil
}

// RestoreBackup replaces path with the backup taken at stamp. The state it
// replaces is backed up first, so a restore can itself be undone; that
// backup is returned, or nil when there was nothing new to save.
func RestoreBackup(path, stamp string) (*Backup, error) {
	list, err := ListBackups(path)
	if err != nil {
		return nil, err
	}
	var source *Backup
	for i := range list {
		if list[i].Stamp == stamp {
			source = &list[i]
		}
	}
	if source == nil {
		return nil, fmt.Errorf("no backup %s of %s", stamp, path)
	}
	data, err := os.ReadFile(source.Path)
	if err != nil {
		return nil, err
	}
	saved, err := BackupFile(path)
	if err != nil {
		return nil, err
	}
	if err := writeFileAtomically(path, data); err != nil {
		return nil, err
	}
	return saved, nil
}
