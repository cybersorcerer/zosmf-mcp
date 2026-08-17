package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

// newTestClients builds a ClientRegistry pointed at a test server.
func newTestClients(t *testing.T, srvURL string) *zosmf.ClientRegistry {
	t.Helper()

	reg := zosmf.NewClientRegistry()
	client, err := zosmf.NewClient(config.SystemConfig{
		Name: "S0W1",
		Host: srvURL,
		Auth: config.AuthConfig{Type: "basic", Username: "test", Password: "test"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	reg.Register("S0W1", client)
	return reg
}

func TestDatasetResourceRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "SYS1.PARMLIB(IEASYS00)") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("SYSCAT=CATALOG\nSYSPLEX=PLEX1\n"))
	}))
	defer srv.Close()

	h := NewDatasetHandler(newTestClients(t, srv.URL))
	uri := DatasetURI("S0W1", "SYS1.PARMLIB(IEASYS00)")

	if !h.Matches(uri) {
		t.Fatal("Matches = false, want true")
	}

	got, err := h.Read(context.Background(), uri)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(got.Text, "SYSPLEX=PLEX1") {
		t.Errorf("Text = %q", got.Text)
	}
	if got.URI != uri {
		t.Errorf("URI = %q, want %q", got.URI, uri)
	}
}

func TestDatasetResourceNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := NewDatasetHandler(newTestClients(t, srv.URL))

	_, err := h.Read(context.Background(), DatasetURI("S0W1", "NO.SUCH.DATA"))
	if err == nil {
		t.Fatal("Read succeeded, want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should say not found, got: %v", err)
	}
}

func TestDatasetResourceAccessDenied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	h := NewDatasetHandler(newTestClients(t, srv.URL))

	_, err := h.Read(context.Background(), DatasetURI("S0W1", "SECRET.DATA"))
	if err == nil {
		t.Fatal("Read succeeded, want error")
	}
	if !strings.Contains(err.Error(), "SAF/RACF") {
		t.Errorf("error should name SAF/RACF, got: %v", err)
	}
	if strings.Contains(err.Error(), srv.URL) {
		t.Error("error leaks the request URL")
	}
}

func TestDatasetResourceUnknownSystem(t *testing.T) {
	h := NewDatasetHandler(zosmf.NewClientRegistry())

	if _, err := h.Read(context.Background(), DatasetURI("NOPE", "USER.DATA")); err == nil {
		t.Error("Read with unknown system succeeded, want error")
	}
}

func TestDatasetTemplates(t *testing.T) {
	h := NewDatasetHandler(zosmf.NewClientRegistry())

	tmpl := h.Templates()
	if len(tmpl) != 1 {
		t.Fatalf("len(Templates()) = %d, want 1", len(tmpl))
	}
	if tmpl[0].URITemplate != "zosmf://{system}/dataset/{dsn}" {
		t.Errorf("URITemplate = %q", tmpl[0].URITemplate)
	}
	if h.Definitions() != nil {
		t.Error("Definitions() should be nil — datasets cannot be enumerated")
	}
}
