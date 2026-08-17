package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

// newToolTestClients builds a ClientRegistry pointed at a test server. It
// must set an Auth block on the SystemConfig — without it, Client.Do
// short-circuits with NO_CREDENTIALS before any HTTP call is made, and a
// test built on top of it would pass without ever exercising the handler.
func newToolTestClients(t *testing.T, srvURL string) *zosmf.ClientRegistry {
	t.Helper()

	reg := zosmf.NewClientRegistry()
	client, err := zosmf.NewClient(config.SystemConfig{
		Name: "S0W1",
		Host: srvURL,
		Auth: config.AuthConfig{Type: "basic", Username: "IBMUSER", Password: "test-only"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	reg.Register("S0W1", client)
	return reg
}

func TestTextOrLinkBelowThreshold(t *testing.T) {
	link := mcp.ResourceLink("zosmf://S0W1/dataset/USER.DATA", "USER.DATA", "", "text/plain", 5)

	got := textOrLink("hello", 100, link)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Type != "text" {
		t.Errorf("Type = %q, want text", got[0].Type)
	}
	if got[0].Text != "hello" {
		t.Errorf("Text = %q, want hello", got[0].Text)
	}
}

func TestTextOrLinkAboveThreshold(t *testing.T) {
	body := strings.Repeat("line of output\n", 1000) // 15000 bytes
	link := mcp.ResourceLink("zosmf://S0W1/dataset/BIG.DATA", "BIG.DATA", "", "text/plain", int64(len(body)))

	got := textOrLink(body, 1000, link)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (preview + link)", len(got))
	}
	if got[0].Type != "text" {
		t.Errorf("first Type = %q, want text", got[0].Type)
	}
	if len(got[0].Text) > previewBytes {
		t.Errorf("preview is %d bytes, want at most %d", len(got[0].Text), previewBytes)
	}
	if got[1].Type != "resource_link" {
		t.Errorf("second Type = %q, want resource_link", got[1].Type)
	}
	if got[1].Size != int64(len(body)) {
		t.Errorf("Size = %d, want %d", got[1].Size, len(body))
	}
}

func TestPreviewCutsAtLineBoundary(t *testing.T) {
	body := strings.Repeat("0123456789\n", 500) // 5500 bytes, 11-byte lines

	got := preview(body)

	if len(got) > previewBytes {
		t.Fatalf("preview is %d bytes, want at most %d", len(got), previewBytes)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("preview does not end at a line boundary: %q", got[len(got)-20:])
	}
}

func TestPreviewOfSingleLongLine(t *testing.T) {
	// No newline to cut at — must still respect the byte cap.
	body := strings.Repeat("x", previewBytes*2)

	got := preview(body)

	if len(got) > previewBytes {
		t.Errorf("preview is %d bytes, want at most %d", len(got), previewBytes)
	}
}

func TestReadDatasetReturnsLinkWhenLarge(t *testing.T) {
	body := strings.Repeat("record of data\n", 4000) // 60000 bytes

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	tool := NewReadDatasetTool(newToolTestClients(t, srv.URL), DefaultInlineMaxBytes)
	args, _ := json.Marshal(map[string]any{"system": "S0W1", "dataset": "BIG.DATA"})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Content) != 2 {
		t.Fatalf("len(Content) = %d, want 2", len(result.Content))
	}
	if result.Content[1].Type != "resource_link" {
		t.Errorf("second content Type = %q, want resource_link", result.Content[1].Type)
	}
	if result.Content[1].URI != "zosmf://S0W1/dataset/BIG.DATA" {
		t.Errorf("URI = %q", result.Content[1].URI)
	}
}

// DefaultInlineMaxBytes is deliberately duplicated in internal/config, which
// must not import this package. Nothing but this test keeps the two halves
// from drifting apart.
func TestDefaultInlineMaxBytesMatchesConfigDefault(t *testing.T) {
	if DefaultInlineMaxBytes != 32768 {
		t.Errorf("DefaultInlineMaxBytes = %d, want 32768", DefaultInlineMaxBytes)
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "systems:\n  - name: SYSA\n    host: https://zosmf.example.com\n    user: USER1\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Resources.InlineMaxBytes != DefaultInlineMaxBytes {
		t.Errorf("config default = %d, tools default = %d — the duplicated constants have drifted",
			cfg.Resources.InlineMaxBytes, DefaultInlineMaxBytes)
	}
}
