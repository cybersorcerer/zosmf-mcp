package setup

import (
	"encoding/json"
	"strings"
	"testing"
)

// The catalog is the contract three consumers share: wizard, flags and the
// JSON output. A field that is missing an id or a help text breaks the
// weakest of them — an agent that has nothing else to go on.
func TestConfigFieldsAreComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range ConfigFields() {
		if f.ID == "" || f.Label == "" || f.Help == "" || f.Group == "" {
			t.Errorf("incomplete field: %+v", f)
		}
		if f.Kind != KindString && f.Kind != KindInt && f.Kind != KindBool {
			t.Errorf("field %s has unknown kind %q", f.ID, f.Kind)
		}
		if f.Cardinality != One && f.Cardinality != Many {
			t.Errorf("field %s has unknown cardinality %q", f.ID, f.Cardinality)
		}
		if seen[f.ID] {
			t.Errorf("duplicate field id %q", f.ID)
		}
		seen[f.ID] = true
	}
	for _, required := range []string{"name", "host", "user", "csi", "zones"} {
		if !seen[required] {
			t.Errorf("catalog is missing %q", required)
		}
	}
}

// A password never belongs in the configuration file — see section 4 of the
// design doc. This is the invariant that keeps a future field from quietly
// breaking that: nothing in the catalog `init config` writes may be named
// after one.
func TestConfigFieldsNameNoPassword(t *testing.T) {
	for _, f := range ConfigFields() {
		if strings.Contains(strings.ToLower(f.ID), "password") || strings.Contains(strings.ToLower(f.YAMLKey), "password") {
			t.Errorf("field %q writes a password into the configuration file", f.ID)
		}
	}
}

func TestCSIAndZonesTakeLists(t *testing.T) {
	for _, f := range ConfigFields() {
		if (f.ID == "csi" || f.ID == "zones") && f.Cardinality != Many {
			t.Errorf("%s must be a list, got %q", f.ID, f.Cardinality)
		}
	}
}

func TestCatalogJSONShape(t *testing.T) {
	data, err := CatalogJSON("init config", ConfigFields())
	if err != nil {
		t.Fatalf("CatalogJSON: %v", err)
	}
	var got struct {
		Command string `json:"command"`
		Fields  []struct {
			ID          string `json:"id"`
			Cardinality string `json:"cardinality"`
			Help        string `json:"help"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got.Command != "init config" {
		t.Errorf("command = %q", got.Command)
	}
	if len(got.Fields) != len(ConfigFields()) {
		t.Errorf("got %d fields, want %d", len(got.Fields), len(ConfigFields()))
	}
}
