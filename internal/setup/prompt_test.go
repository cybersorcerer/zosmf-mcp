package setup

import (
	"bytes"
	"strings"
	"testing"
)

func TestAskAllCollectsAnswers(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		"MAINT",                       // name
		"https://zos.example.com:443", // host
		"",                            // port, skipped
		"IBMUSER",                     // user
		"A.CSI, B.CSI",                // csi
		"MVST100",                     // zones
		"",                            // default-zones
		"", "", "", "", "",            // the remaining optional fields
	}, "\n") + "\n")

	v := NewValues()
	var out bytes.Buffer
	if err := AskAll(ConfigFields(), v, in, &out); err != nil {
		t.Fatalf("AskAll: %v", err)
	}

	if v.One("name") != "MAINT" {
		t.Errorf("name = %q", v.One("name"))
	}
	if got := v.Many("csi"); len(got) != 2 || got[1] != "B.CSI" {
		t.Errorf("csi = %v, want two entries", got)
	}
	if v.Has("port") {
		t.Error("an empty answer must not become a value")
	}
}

// A required field that is left empty is asked again rather than accepted.
func TestAskAllRepeatsRequiredFields(t *testing.T) {
	in := strings.NewReader("\nMAINT\nhttps://zos.example.com:443\n\nIBMUSER\n\n\n\n\n\n\n\n")
	v := NewValues()
	var out bytes.Buffer
	if err := AskAll(ConfigFields(), v, in, &out); err != nil {
		t.Fatalf("AskAll: %v", err)
	}
	if v.One("name") != "MAINT" {
		t.Errorf("name = %q, want the answer after the empty one", v.One("name"))
	}
	if !strings.Contains(out.String(), "required") {
		t.Errorf("the user was not told the field is required:\n%s", out.String())
	}
}

// The consequence of skipping csi and zones is spelled out, because the
// failure otherwise appears much later as a tool error.
func TestAskAllWarnsWhenSMPEFieldsAreSkipped(t *testing.T) {
	in := strings.NewReader("MAINT\nhttps://zos.example.com:443\n\nIBMUSER\n\n\n\n\n\n\n\n\n")
	v := NewValues()
	var out bytes.Buffer
	if err := AskAll(ConfigFields(), v, in, &out); err != nil {
		t.Fatalf("AskAll: %v", err)
	}
	if !strings.Contains(out.String(), "SMP/E") {
		t.Errorf("no warning about the SMP/E tools:\n%s", out.String())
	}
}
