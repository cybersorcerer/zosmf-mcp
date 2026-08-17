package resources

import (
	"context"
	"errors"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

// stubHandler serves one fixed URI.
type stubHandler struct {
	uri  string
	body string
}

func (s *stubHandler) Definitions() []mcp.Resource {
	return []mcp.Resource{{URI: s.uri, Name: "stub"}}
}

func (s *stubHandler) Templates() []mcp.ResourceTemplate { return nil }

func (s *stubHandler) Matches(uri string) bool { return uri == s.uri }

func (s *stubHandler) Read(ctx context.Context, uri string) (mcp.ResourceContents, error) {
	return mcp.ResourceContents{URI: uri, MIMEType: "text/plain", Text: s.body}, nil
}

func TestRegistryReadRoutesToHandler(t *testing.T) {
	r := NewResourceRegistry()
	if err := r.Register(&stubHandler{uri: "zosmf:///systems", body: "hello"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := r.Read(context.Background(), "zosmf:///systems")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Text != "hello" {
		t.Errorf("Text = %q, want hello", got.Text)
	}
}

func TestRegistryUnknownURI(t *testing.T) {
	r := NewResourceRegistry()

	_, err := r.Read(context.Background(), "zosmf://S0W1/dataset/USER.DATA")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRegistryRejectsDuplicateURI(t *testing.T) {
	r := NewResourceRegistry()
	if err := r.Register(&stubHandler{uri: "zosmf:///systems"}); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register(&stubHandler{uri: "zosmf:///systems"}); err == nil {
		t.Error("second Register succeeded, want error")
	}
}

func TestRegistryListAndTemplates(t *testing.T) {
	r := NewResourceRegistry()
	_ = r.Register(&stubHandler{uri: "zosmf:///systems"})

	if got := len(r.List()); got != 1 {
		t.Errorf("len(List()) = %d, want 1", got)
	}
	if got := len(r.Templates()); got != 0 {
		t.Errorf("len(Templates()) = %d, want 0", got)
	}
}

func TestRegisterAllWiresEveryHandler(t *testing.T) {
	r := NewResourceRegistry()
	cfg := &config.Config{
		Systems: []config.SystemConfig{{Name: "S0W1", Host: "https://zosmf.example.com:443"}},
	}

	if err := RegisterAll(r, zosmf.NewClientRegistry(), cfg); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	if got := len(r.List()); got != 1 {
		t.Errorf("len(List()) = %d, want 1 (the system inventory)", got)
	}
	if got := len(r.Templates()); got != 3 {
		t.Errorf("len(Templates()) = %d, want 3 (dataset, uss, spool)", got)
	}
}
