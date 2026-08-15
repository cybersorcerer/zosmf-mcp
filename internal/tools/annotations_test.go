package tools

import (
	"reflect"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

func TestClientRegistrySystemNames_SortedAndUppercased(t *testing.T) {
	reg := zosmf.NewClientRegistry()
	for _, name := range []string{"zeta", "alpha", "Mike"} {
		client, err := zosmf.NewClient(config.SystemConfig{Name: name, Host: "https://example.invalid"})
		if err != nil {
			t.Fatal(err)
		}
		reg.Register(name, client)
	}
	want := []string{"ALPHA", "MIKE", "ZETA"}
	for i := 0; i < 5; i++ {
		if got := reg.SystemNames(); !reflect.DeepEqual(got, want) {
			t.Fatalf("SystemNames() = %v, want %v (call %d)", got, want, i)
		}
	}
}

func TestSSHClientRegistrySystemNames_SortedAndUppercased(t *testing.T) {
	reg := zosmf.NewSSHClientRegistry([]config.SSHSystemConfig{
		{Name: "zoau2"}, {Name: "Aoau1"},
	})
	want := []string{"AOAU1", "ZOAU2"}
	if got := reg.SystemNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("SystemNames() = %v, want %v", got, want)
	}
}

func TestSystemProperty_AddsEnumWhenSystemsExist(t *testing.T) {
	prop := systemProperty([]string{"PROD", "TEST"})
	if prop["type"] != "string" {
		t.Errorf("type = %v", prop["type"])
	}
	enum, ok := prop["enum"].([]string)
	if !ok || !reflect.DeepEqual(enum, []string{"PROD", "TEST"}) {
		t.Errorf("enum = %v, want [PROD TEST]", prop["enum"])
	}
	if prop["description"] == "" {
		t.Error("description must not be empty")
	}
}

func TestSystemProperty_OmitsEnumWhenNoSystems(t *testing.T) {
	prop := systemProperty(nil)
	if _, ok := prop["enum"]; ok {
		t.Error("enum must be omitted when no systems are configured — an empty enum matches nothing")
	}
}

func TestSSHSystemProperty_DescribesTheSSHNamespace(t *testing.T) {
	prop := sshSystemProperty([]string{"LPAR1"})
	desc, _ := prop["description"].(string)
	if desc == "" {
		t.Fatal("description must not be empty")
	}
	// The whole point of the separate helper: it must name ssh_systems,
	// so the model cannot confuse it with the z/OSMF systems: block.
	if !strings.Contains(desc, "ssh_systems") {
		t.Errorf("description must mention ssh_systems, got %q", desc)
	}
}

func TestReadOnlyTool_SetsHints(t *testing.T) {
	a := readOnlyTool("List Datasets")
	if a.Title != "List Datasets" {
		t.Errorf("Title = %q", a.Title)
	}
	if !a.ReadOnlyHint || a.DestructiveHint || !a.IdempotentHint || a.OpenWorldHint {
		t.Errorf("unexpected hints: %+v", a)
	}
}

func TestWriteTool_SetsHints(t *testing.T) {
	a := writeTool("Delete Dataset", true, true, false)
	if a.ReadOnlyHint {
		t.Error("ReadOnlyHint must be false for a write tool")
	}
	if !a.DestructiveHint || !a.IdempotentHint || a.OpenWorldHint {
		t.Errorf("unexpected hints: %+v", a)
	}
}
