package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

func TestJobSpoolResourceRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/files/2/records") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("IEF142I MYJOB GO - STEP WAS EXECUTED\n"))
	}))
	defer srv.Close()

	h := NewJobSpoolHandler(newTestClients(t, srv.URL))
	uri := SpoolURI("S0W1", "MYJOB", "JOB12345", 2)

	if !h.Matches(uri) {
		t.Fatal("Matches = false, want true")
	}

	got, err := h.Read(context.Background(), uri)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(got.Text, "IEF142I") {
		t.Errorf("Text = %q", got.Text)
	}
}

func TestJobSpoolResourceNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := NewJobSpoolHandler(newTestClients(t, srv.URL))

	_, err := h.Read(context.Background(), SpoolURI("S0W1", "MYJOB", "JOB99999", 1))
	if err == nil {
		t.Fatal("Read succeeded, want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should say not found, got: %v", err)
	}
}

func TestJobSpoolTemplates(t *testing.T) {
	h := NewJobSpoolHandler(zosmf.NewClientRegistry())

	tmpl := h.Templates()
	if len(tmpl) != 1 {
		t.Fatalf("len(Templates()) = %d, want 1", len(tmpl))
	}
	if tmpl[0].URITemplate != "zosmf://{system}/job/{jobname}/{jobid}/spool/{id}" {
		t.Errorf("URITemplate = %q", tmpl[0].URITemplate)
	}
}
