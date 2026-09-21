package setup

import (
	"flag"
	"io"
	"strings"
	"testing"
)

func parse(t *testing.T, args ...string) *Values {
	t.Helper()
	fs := flag.NewFlagSet("init config", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	v := BindFlags(fs, ConfigFields())
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return v
}

func TestScalarFlag(t *testing.T) {
	v := parse(t, "--name", "MAINT")
	if got := v.One("name"); got != "MAINT" {
		t.Errorf("name = %q", got)
	}
}

// Both spellings have to work: an agent copying the documented example uses
// one, a human at a shell often the other.
func TestListFlagAcceptsRepeatAndCommas(t *testing.T) {
	repeated := parse(t, "--csi", "A.CSI", "--csi", "B.CSI").Many("csi")
	commas := parse(t, "--csi", "A.CSI,B.CSI").Many("csi")

	for _, got := range [][]string{repeated, commas} {
		if len(got) != 2 || got[0] != "A.CSI" || got[1] != "B.CSI" {
			t.Errorf("csi = %v, want [A.CSI B.CSI]", got)
		}
	}
}

func TestValidateNamesTheMissingFlag(t *testing.T) {
	err := Validate(ConfigFields(), parse(t, "--name", "MAINT"))
	if err == nil {
		t.Fatal("expected an error when host and user are missing")
	}
	for _, want := range []string{"--host", "--user"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
	// A small model correcting itself needs the example, not just the name.
	if !strings.Contains(err.Error(), "IBMUSER") {
		t.Errorf("error %q carries no example", err)
	}
}

// Flags are generated from the catalog, and this test is what keeps that
// true: a field added to one and not the other is a field an agent is told
// about but cannot pass.
func TestEveryCatalogFieldHasAFlag(t *testing.T) {
	fs := flag.NewFlagSet("init config", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	BindFlags(fs, ConfigFields())

	for _, f := range ConfigFields() {
		if fs.Lookup(f.ID) == nil {
			t.Errorf("catalog field %q has no flag", f.ID)
		}
	}
	count := 0
	fs.VisitAll(func(*flag.Flag) { count++ })
	if count != len(ConfigFields()) {
		t.Errorf("%d flags for %d fields — one side has an extra", count, len(ConfigFields()))
	}
}

func TestValidatePassesWhenRequiredArePresent(t *testing.T) {
	v := parse(t, "--name", "MAINT", "--host", "https://zos.example.com:443", "--user", "IBMUSER")
	if err := Validate(ConfigFields(), v); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
