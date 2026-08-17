package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

func TestUSSResourceRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/etc/profile") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("export PATH=/bin\n"))
	}))
	defer srv.Close()

	h := NewUSSHandler(newTestClients(t, srv.URL))
	uri := USSURI("S0W1", "/etc/profile")

	if !h.Matches(uri) {
		t.Fatal("Matches = false, want true")
	}

	got, err := h.Read(context.Background(), uri)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(got.Text, "export PATH=/bin") {
		t.Errorf("Text = %q", got.Text)
	}
}

func TestUSSResourceKeepsPathCase(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	h := NewUSSHandler(newTestClients(t, srv.URL))

	if _, err := h.Read(context.Background(), USSURI("S0W1", "/u/ibmuser/MyScript.sh")); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(seen, "MyScript.sh") {
		t.Errorf("USS path was case-folded: %s", seen)
	}
}

func TestUSSTemplates(t *testing.T) {
	h := NewUSSHandler(zosmf.NewClientRegistry())

	tmpl := h.Templates()
	if len(tmpl) != 1 {
		t.Fatalf("len(Templates()) = %d, want 1", len(tmpl))
	}
	if tmpl[0].URITemplate != "zosmf://{system}/uss/{path}" {
		t.Errorf("URITemplate = %q", tmpl[0].URITemplate)
	}
}
