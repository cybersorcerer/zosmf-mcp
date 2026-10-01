package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lines(ls ...string) *strings.Reader {
	return strings.NewReader(strings.Join(ls, "\n") + "\n")
}

// newSystem answers every question for a fresh MAINT with two CSIs and one
// zone, and accepts the defaults everywhere else.
var newSystem = []string{
	"n",
	"MAINT", "https://zos.example.com:443", "IBMUSER",
	"",                   // verify TLS: default yes
	"",                   // ca-file
	"A.CSI", "B.CSI", "", // CSIs
	"MVST100", "", // zones
	"", "", // TSO account, proc
	"", // other tools: no
	"", // ZOAU: no
	"", // write: yes
}

func runWizard(t *testing.T, path string, dryRun bool, in *strings.Reader) string {
	t.Helper()
	var out bytes.Buffer
	if err := RunWizard(path, filepath.Join(filepath.Dir(path), ".env"), dryRun, in, &out); err != nil {
		t.Fatalf("RunWizard: %v\n%s", err, out.String())
	}
	return out.String()
}

func TestWizardWritesANewSystem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	out := runWizard(t, path, false, lines(append(newSystem, "d")...))

	cfg, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("nothing written: %v\n%s", err, out)
	}
	for _, want := range []string{"name: MAINT", "- A.CSI\n", "- B.CSI\n", "- MVST100", "rejectUnauthorized: true", "transport:"} {
		if !strings.Contains(string(cfg), want) {
			t.Errorf("file lacks %q:\n%s", want, cfg)
		}
	}
	if !strings.Contains(out, "→ 2 CSIs: A.CSI, B.CSI") {
		t.Errorf("no recap of the CSIs:\n%s", out)
	}
	if !strings.Contains(out, "init env") {
		t.Errorf("no pointer to init env:\n%s", out)
	}
}

func TestWizardDefinesTwoSystemsInOneRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	second := append([]string{}, newSystem...)
	second[1] = "PROD"
	runWizard(t, path, false, lines(append(append(newSystem, second...), "d")...))

	doc, _ := LoadDocument(path)
	if got := doc.Systems(); len(got) != 2 || got[1].Name != "PROD" {
		t.Errorf("systems = %+v, want MAINT and PROD", got)
	}
}

// Editing: Enter keeps every value, a CSI is added after the existing
// ones, and keys other tools wrote stay.
func TestWizardEditKeepsAndAdds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	_ = os.WriteFile(path, []byte(`servers:
  - name: MAINT
    host: https://zos.example.com:443
    user: IBMUSER
    csi:
      - A.CSI
    zones:
      - MVST100
    profile: kept-for-vscode
`), 0o600)

	out := runWizard(t, path, false, lines(
		"1",
		"", "", // host, user kept
		"", "", // verify, ca-file
		"a", "B.CSI", "", // CSIs: add one
		"", // zones: keep
		"", "", "", "", "",
		"d",
	))

	cfg, _ := os.ReadFile(path)
	for _, want := range []string{"user: IBMUSER", "- A.CSI\n      - B.CSI", "- MVST100", "profile: kept-for-vscode"} {
		if !strings.Contains(string(cfg), want) {
			t.Errorf("file lacks %q:\n%s", want, cfg)
		}
	}
	if !strings.Contains(out, "IBMUSER]") {
		t.Errorf("the current value is not offered in brackets:\n%s", out)
	}
	if !strings.Contains(out, "CSI 2:") {
		t.Errorf("adding did not continue the numbering:\n%s", out)
	}
}

func TestWizardDeclinedWriteChangesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	answers := append([]string{}, newSystem...)
	answers[len(answers)-1] = "n"
	runWizard(t, path, false, lines(append(answers, "d")...))
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("a declined write created the file")
	}
}

func TestWizardBacksUpAtStartAndRestores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	original := "servers:\n  - name: OLD\n    host: https://old:443\n    user: U\n"
	_ = os.WriteFile(path, []byte(original), 0o600)

	out := runWizard(t, path, false, lines(append(newSystem, "d")...))
	if !strings.Contains(out, "Backup of the current file: "+filepath.Join(filepath.Dir(path), BackupDirName)) {
		t.Errorf("the start backup is not shown:\n%s", out)
	}

	// Backup 1 is the one this second start just took — the current file —
	// and is marked so; backup 2 is the original.
	out = runWizard(t, path, false, lines("r", "2", "d"))
	if !strings.Contains(out, "(= current file)") {
		t.Errorf("the backup identical to the file is not marked:\n%s", out)
	}
	if got, _ := os.ReadFile(path); string(got) != original {
		t.Errorf("after restoring the original the file is\n%s", got)
	}
}

func TestWizardZOAUGateWritesSSHSystem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	answers := append([]string{}, newSystem[:len(newSystem)-2]...)
	answers = append(answers,
		"y",                         // ZOAU
		"",                          // ssh-host: taken from the URL
		"",                          // ssh-port: 22
		"ibmuser",                   // ssh-user
		"/home/ibmuser/.ssh/id_rsa", // key
		"/usr/lpp/IBM/zoautil",      // zoau-path
		"",                          // write
		"d",
	)
	runWizard(t, path, false, lines(answers...))
	cfg, _ := os.ReadFile(path)
	for _, want := range []string{"ssh_systems:", "host: zos.example.com\n", "port: 22", "ZOAU_HOME: /usr/lpp/IBM/zoautil"} {
		if !strings.Contains(string(cfg), want) {
			t.Errorf("file lacks %q:\n%s", want, cfg)
		}
	}
}

func TestWizardDryRunWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	out := runWizard(t, path, true, lines(append(newSystem, "d")...))
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("--print wrote the file")
	}
	if !strings.Contains(out, "name: MAINT") {
		t.Errorf("--print did not show the result:\n%s", out)
	}
}

func TestWizardRequiredFieldIsAskedAgain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	answers := append([]string{"n", ""}, newSystem[1:]...)
	out := runWizard(t, path, true, lines(append(answers, "d")...))
	if !strings.Contains(out, "is required") {
		t.Errorf("an empty required answer was accepted:\n%s", out)
	}
}
