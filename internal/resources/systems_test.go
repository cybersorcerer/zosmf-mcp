package resources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

func TestSystemsHandlerListsConfiguredSystems(t *testing.T) {
	cfg := &config.Config{
		Systems: []config.SystemConfig{
			{Name: "S0W1", Host: "https://zosmf.example.com:443", CSIDatasets: []string{"SYS1.SMPE.CSI"}},
			{Name: "S0W2", Host: "https://zosmf2.example.com:443"},
		},
	}
	clients := zosmf.NewClientRegistry()

	h := NewSystemsHandler(clients, cfg)

	defs := h.Definitions()
	if len(defs) != 1 {
		t.Fatalf("len(Definitions()) = %d, want 1", len(defs))
	}
	if defs[0].URI != SystemsURI {
		t.Errorf("URI = %q, want %q", defs[0].URI, SystemsURI)
	}

	if !h.Matches(SystemsURI) {
		t.Error("Matches(SystemsURI) = false, want true")
	}
	if h.Matches("zosmf://S0W1/dataset/USER.DATA") {
		t.Error("Matches claimed a dataset URI")
	}

	got, err := h.Read(context.Background(), SystemsURI)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.MIMEType != "application/json" {
		t.Errorf("MIMEType = %q, want application/json", got.MIMEType)
	}

	var payload struct {
		Systems []struct {
			Name        string   `json:"name"`
			Host        string   `json:"host"`
			CSIDatasets []string `json:"csi_datasets,omitempty"`
		} `json:"systems"`
	}
	if err := json.Unmarshal([]byte(got.Text), &payload); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if len(payload.Systems) != 2 {
		t.Fatalf("len(systems) = %d, want 2", len(payload.Systems))
	}
	if payload.Systems[0].Name != "S0W1" {
		t.Errorf("first system = %q, want S0W1", payload.Systems[0].Name)
	}
	if strings.Contains(got.Text, "password") {
		t.Error("payload leaks credential fields")
	}
}

func TestSystemsHandlerRejectsOtherURI(t *testing.T) {
	h := NewSystemsHandler(zosmf.NewClientRegistry(), &config.Config{})

	if _, err := h.Read(context.Background(), "zosmf://S0W1/dataset/USER.DATA"); err == nil {
		t.Error("Read of a foreign URI succeeded, want error")
	}
}
