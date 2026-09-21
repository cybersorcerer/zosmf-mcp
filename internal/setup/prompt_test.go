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
	if err := AskAll(ConfigFields(), SSHFields(), v, in, &out); err != nil {
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
	if err := AskAll(ConfigFields(), SSHFields(), v, in, &out); err != nil {
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
	if err := AskAll(ConfigFields(), SSHFields(), v, in, &out); err != nil {
		t.Fatalf("AskAll: %v", err)
	}
	if !strings.Contains(out.String(), "SMP/E") {
		t.Errorf("no warning about the SMP/E tools:\n%s", out.String())
	}
}

// An empty answer at a field with a default must still take it — a user
// pressing return at reject-unauthorized has to end up with true, not
// nothing.
func TestAskAllAppliesDefaultOnEmptyAnswer(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		"MAINT", "https://zos.example.com:443", "", "IBMUSER",
		"", "", "", "", "", // csi, zones, default-zones, tso-account, tso-proc
		"",     // reject-unauthorized: press return, expect the catalog default
		"", "", // ca-file, home
	}, "\n") + "\n")
	v := NewValues()
	var out bytes.Buffer
	if err := AskAll(ConfigFields(), SSHFields(), v, in, &out); err != nil {
		t.Fatalf("AskAll: %v", err)
	}
	if got := v.One("reject-unauthorized"); got != "true" {
		t.Errorf("reject-unauthorized = %q, want the catalog default %q", got, "true")
	}
}

// Saying yes to the ZOAU gate walks that group's questions too; saying
// nothing (the default, "no") must not ask them at all.
func TestAskAllOffersTheZOAUGroupOnlyOnYes(t *testing.T) {
	systemAnswers := []string{
		"MAINT", "https://zos.example.com:443", "", "IBMUSER",
		"", "", "", "", "", "", "", "",
	}

	t.Run("no", func(t *testing.T) {
		in := strings.NewReader(strings.Join(append(append([]string{}, systemAnswers...), "n"), "\n") + "\n")
		v := NewValues()
		var out bytes.Buffer
		if err := AskAll(ConfigFields(), SSHFields(), v, in, &out); err != nil {
			t.Fatalf("AskAll: %v", err)
		}
		if v.Has("zoau-path") {
			t.Error("answering no to the ZOAU gate must not ask its fields")
		}
	})

	t.Run("yes", func(t *testing.T) {
		zoauAnswers := []string{
			"y",
			"zos.example.com",           // ssh-host
			"",                          // ssh-port, skipped
			"ibmuser",                   // ssh-user
			"/home/ibmuser/.ssh/id_rsa", // ssh-key-file
			"/usr/lpp/IBM/zoautil",      // zoau-path
		}
		in := strings.NewReader(strings.Join(append(append([]string{}, systemAnswers...), zoauAnswers...), "\n") + "\n")
		v := NewValues()
		var out bytes.Buffer
		if err := AskAll(ConfigFields(), SSHFields(), v, in, &out); err != nil {
			t.Fatalf("AskAll: %v", err)
		}
		if v.One("zoau-path") != "/usr/lpp/IBM/zoautil" {
			t.Errorf("zoau-path = %q, want the answer given after saying yes", v.One("zoau-path"))
		}
	})
}
