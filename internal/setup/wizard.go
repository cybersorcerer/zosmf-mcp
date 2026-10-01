package setup

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var errInputEnded = errors.New("input ended before the setup was finished")

// wizard is the interactive `init config`: a menu over the file's systems,
// one grouped walk per system, a preview, and a write after each system.
type wizard struct {
	path, envPath string
	dryRun        bool
	fields, ssh   []Field
	doc           *Document
	in            *bufio.Reader
	out           io.Writer
	wrote         bool
}

// RunWizard runs the interactive setup on the configuration at path. With
// dryRun nothing is written and the result is printed at the end. envPath
// is only used to offer its backups for restoring.
func RunWizard(path, envPath string, dryRun bool, in io.Reader, out io.Writer) error {
	doc, err := LoadDocument(path)
	if err != nil {
		return err
	}
	w := &wizard{
		path: path, envPath: envPath, dryRun: dryRun,
		fields: ConfigFields(), ssh: SSHFields(),
		doc: doc, in: bufio.NewReader(in), out: out,
	}

	w.printf("zosmf-mcp setup — %s\n", path)
	if !dryRun {
		if err := w.backupAtStart(); err != nil {
			return err
		}
	}

	for {
		systems := w.doc.Systems()
		w.printf("\nSystems in this file:\n")
		if len(systems) == 0 {
			w.printf("  (none yet)\n")
		}
		for i, s := range systems {
			w.printf("  %d  %-10s %s\n", i+1, s.Name, s.Host)
		}
		menu := "\n[n] new system"
		switch len(systems) {
		case 0:
		case 1:
			menu += "  [1] edit system"
		default:
			menu += fmt.Sprintf("  [1-%d] edit system", len(systems))
		}
		if !dryRun {
			menu += "  [r] restore a backup"
		}
		w.printf("%s  [d] done\n> ", menu)

		answer, err := w.read()
		if errors.Is(err, errInputEnded) {
			return w.finish()
		}
		if err != nil {
			return err
		}
		n, numErr := strconv.Atoi(answer)
		switch {
		case strings.EqualFold(answer, "n"):
			err = w.editSystem("")
		case numErr == nil && n >= 1 && n <= len(systems):
			err = w.editSystem(systems[n-1].Name)
		case strings.EqualFold(answer, "r") && !dryRun:
			err = w.restore()
		case strings.EqualFold(answer, "d"):
			return w.finish()
		default:
			w.printf("  %q is not one of the choices.\n", answer)
		}
		if err != nil {
			return err
		}
	}
}

func (w *wizard) backupAtStart() error {
	if _, err := os.Stat(w.path); os.IsNotExist(err) {
		w.printf("New file — nothing to back up yet.\n")
		return nil
	}
	if _, err := BackupFile(w.path); err != nil {
		return err
	}
	// BackupFile skips a file identical to its newest backup, so the newest
	// backup is the one that holds the file as it is now either way.
	list, err := ListBackups(w.path)
	if err != nil {
		return err
	}
	if len(list) > 0 {
		w.printf("Backup of the current file: %s\n", list[0].Path)
	}
	return nil
}

func (w *wizard) finish() error {
	if w.dryRun {
		out, err := w.doc.Bytes()
		if err != nil {
			return err
		}
		w.printf("\n--print: nothing was written. The file would read:\n\n%s", out)
		return nil
	}
	if w.wrote {
		w.printf("\nPasswords are not stored in this file. Next: zosmf-mcp init env --file %s\n", w.path)
	}
	return nil
}

// editSystem walks one system, new when name is empty, and writes it after
// a preview the user confirms.
func (w *wizard) editSystem(name string) error {
	v := NewValues()
	if name != "" {
		v = w.doc.SystemValues(name, w.fields, w.ssh)
		v.Set("name", []string{name})
		w.printf("\nEditing %s — Enter keeps the value in brackets.\n", name)
	}

	w.section("Connection")
	if name == "" {
		if err := w.askScalar(w.field("name"), v); err != nil {
			return err
		}
		name = v.One("name")
		if w.doc.entry(w.systemsKey(), name) != nil {
			w.printf("  %s already exists — editing it. Enter keeps the value in brackets.\n", name)
			v = w.doc.SystemValues(name, w.fields, w.ssh)
			v.Set("name", []string{name})
		}
	}
	if err := w.askScalar(w.field("host"), v); err != nil {
		return err
	}
	if u, err := url.Parse(v.One("host")); err == nil && u.Port() == "" {
		if err := w.askScalar(w.field("port"), v); err != nil {
			return err
		}
	}
	if err := w.askScalar(w.field("user"), v); err != nil {
		return err
	}
	if err := w.askBool(w.field("reject-unauthorized"), v); err != nil {
		return err
	}
	if v.One("reject-unauthorized") == "true" {
		if err := w.askScalar(w.field("ca-file"), v); err != nil {
			return err
		}
	}

	w.section("SMP/E")
	if err := w.askList(w.field("csi"), v, "CSI", "The first CSI is the default when a call names none."); err != nil {
		return err
	}
	if err := w.askList(w.field("zones"), v, "zone", "All zones below are the default when a call names none."); err != nil {
		return err
	}
	if !v.Has("csi") || !v.Has("zones") {
		w.printf("  Note: without a CSI and zones every SMP/E tool call has to name them.\n")
	}

	w.section("TSO")
	for _, id := range []string{"tso-account", "tso-proc"} {
		if err := w.askScalar(w.field(id), v); err != nil {
			return err
		}
	}

	other, err := w.gate("Settings for other tools (vscode-smpe, nvpdf)?", v.Has("default-zones") || v.Has("home"))
	if err != nil {
		return err
	}
	if other {
		if err := w.askList(w.field("default-zones"), v, "zone", ""); err != nil {
			return err
		}
		if err := w.askScalar(w.field("home"), v); err != nil {
			return err
		}
	}

	zoau, err := w.gate("Configure ZOAU tools? They run over SSH, not z/OSMF.", v.Has("zoau-path"))
	if err != nil {
		return err
	}
	if zoau {
		for _, f := range w.ssh {
			if f.ID == "ssh-host" && !v.Has(f.ID) {
				if u, err := url.Parse(v.One("host")); err == nil {
					f.Default = u.Hostname()
				}
			}
			if err := w.askScalar(f, v); err != nil {
				return err
			}
		}
	}

	return w.confirmAndWrite(name, v, zoau)
}

func (w *wizard) confirmAndWrite(name string, v *Values, zoau bool) error {
	before, err := w.doc.Bytes()
	if err != nil {
		return err
	}
	entries, err := EntriesFor(w.fields, v)
	if err != nil {
		return err
	}
	if _, err := w.doc.UpsertSystem(name, entries); err != nil {
		return err
	}
	if zoau {
		sshEntries, err := EntriesFor(w.ssh, v)
		if err != nil {
			return err
		}
		sshEntries = append(sshEntries, ZOAUEnvironment(v.One("zoau-path"))...)
		if _, err := w.doc.UpsertSSHSystem(name, sshEntries); err != nil {
			return err
		}
	}
	if _, err := w.doc.EnsureDefaultTransport(); err != nil {
		return err
	}

	preview, err := w.doc.Preview(name)
	if err != nil {
		return err
	}
	w.section("Summary")
	w.printf("%s", preview)
	write, err := w.gate("Write?", true)
	if err != nil {
		return err
	}
	if !write {
		w.doc, err = parseDocument(before, w.path)
		w.printf("  Discarded — the file is unchanged.\n")
		return err
	}
	if w.dryRun {
		w.printf("  Kept for the --print output.\n")
		return nil
	}
	backup, err := w.doc.Save(w.path)
	if err != nil {
		return err
	}
	w.wrote = true
	w.printf("  Wrote %s\n", w.path)
	if backup != nil {
		w.printf("  Previous version saved as %s\n", backup.Path)
	}
	return nil
}

func (w *wizard) restore() error {
	type labelled struct {
		target string
		Backup
	}
	var all []labelled
	for _, t := range []struct{ name, path string }{{"config", w.path}, {"env", w.envPath}} {
		list, err := ListBackups(t.path)
		if err != nil {
			return err
		}
		for _, b := range list {
			all = append(all, labelled{t.name, b})
		}
	}
	if len(all) == 0 {
		w.printf("  No backups yet.\n")
		return nil
	}
	w.printf("\nBackups, newest first:\n")
	for i, b := range all {
		mark := ""
		saved, err1 := os.ReadFile(b.Path)
		current, err2 := os.ReadFile(b.File)
		if err1 == nil && err2 == nil && bytes.Equal(saved, current) {
			mark = "  (= current file)"
		}
		w.printf("  %d  %-6s %s  %s%s\n", i+1, b.target, b.Stamp, b.File, mark)
	}
	w.printf("Restore which? Enter cancels.\n> ")
	answer, err := w.read()
	if err != nil || answer == "" {
		return err
	}
	n, err := strconv.Atoi(answer)
	if err != nil || n < 1 || n > len(all) {
		w.printf("  %q is not one of the backups.\n", answer)
		return nil
	}
	b := all[n-1]
	saved, err := RestoreBackup(b.File, b.Stamp)
	if err != nil {
		return err
	}
	w.printf("  Restored %s from the backup of %s.\n", b.File, b.Stamp)
	if saved != nil {
		w.printf("  The version it replaced is saved as %s\n", saved.Path)
	}
	if b.target == "config" {
		w.doc, err = LoadDocument(w.path)
	}
	return err
}

// askScalar asks one value. The current value — or the field's default —
// is shown in brackets and kept on Enter.
func (w *wizard) askScalar(f Field, v *Values) error {
	current := v.One(f.ID)
	if current == "" {
		current = f.Default
	}
	w.printf("  %s\n", f.Help)
	for {
		label := f.Label
		if current != "" {
			label += " [" + current + "]"
		}
		w.printf("%s: ", label)
		answer, err := w.read()
		if err != nil {
			return err
		}
		switch {
		case answer == "" && current != "":
			v.Set(f.ID, []string{current})
		case answer == "" && f.Required:
			w.printf("  %s is required.\n", f.Label)
			continue
		case answer == "":
		case f.Kind == KindInt && !isNumber(answer):
			w.printf("  %q is not a number.\n", answer)
			continue
		default:
			v.Set(f.ID, []string{answer})
		}
		return nil
	}
}

func isNumber(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func (w *wizard) askBool(f Field, v *Values) error {
	current := v.One(f.ID)
	if current == "" {
		current = f.Default
	}
	w.printf("  %s\n", f.Help)
	yes, err := w.gate(f.Label+"?", current == "true")
	if err != nil {
		return err
	}
	v.Set(f.ID, []string{strconv.FormatBool(yes)})
	return nil
}

// askList takes a list one entry per line, because a comma separated line
// hid how many entries there were and which one came first. An existing
// list can be kept, replaced or added to.
func (w *wizard) askList(f Field, v *Values, item, explain string) error {
	if explain != "" {
		w.printf("  %s\n", explain)
	}
	var vals []string
	if current := v.Many(f.ID); len(current) > 0 {
		w.printf("%s: %s\n[k] keep  [r] replace  [a] add  > ", f.Label, strings.Join(current, ", "))
		answer, err := w.read()
		if err != nil {
			return err
		}
		switch strings.ToLower(answer) {
		case "r":
		case "a":
			vals = current
		default:
			return nil
		}
	}
	w.printf("%s — one per line, an empty line ends:\n", f.Label)
	added := false
	for i := len(vals) + 1; ; i++ {
		w.printf("  %s %d: ", item, i)
		answer, err := w.read()
		if err != nil {
			return err
		}
		if answer == "" {
			break
		}
		vals = append(vals, answer)
		added = true
	}
	if !added {
		if v.Has(f.ID) {
			w.printf("  Nothing entered — keeping %s.\n", strings.Join(v.Many(f.ID), ", "))
		}
		return nil
	}
	v.Set(f.ID, vals)
	noun := item
	if len(vals) != 1 {
		noun += "s"
	}
	w.printf("  → %d %s: %s\n", len(vals), noun, strings.Join(vals, ", "))
	return nil
}

// gate asks a yes/no question; Enter takes the default.
func (w *wizard) gate(question string, defaultYes bool) (bool, error) {
	hint := "[y/N]"
	if defaultYes {
		hint = "[Y/n]"
	}
	for {
		w.printf("%s %s ", question, hint)
		answer, err := w.read()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "":
			return defaultYes, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		w.printf("  Answer y or n.\n")
	}
}

func (w *wizard) read() (string, error) {
	line, err := w.in.ReadString('\n')
	if err != nil && line == "" {
		return "", errInputEnded
	}
	return strings.TrimSpace(line), nil
}

func (w *wizard) section(title string) {
	w.printf("\n── %s ──\n", title)
}

func (w *wizard) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(w.out, format, args...)
}

func (w *wizard) field(id string) Field {
	for _, f := range w.fields {
		if f.ID == id {
			return f
		}
	}
	panic("setup: no field " + id)
}

func (w *wizard) systemsKey() string {
	key, _ := w.doc.systemsKey()
	return key
}
