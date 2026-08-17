package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/resources"
)

// TestSpoolLinkIsReadableAsResource closes the seam between internal/tools and
// internal/resources: the link a tool emits must be a URI the resource
// registry can resolve back to the full content. Both halves are exercised
// against the same httptest server, so a URI change on one side that the
// other does not learn about fails here rather than in production.
//
// This is the design's binding success criterion end to end.
func TestSpoolLinkIsReadableAsResource(t *testing.T) {
	jesysmsg := strings.Repeat("J", 2*1024)
	jesjcl := strings.Repeat("C", 4*1024)
	sysprint := strings.Repeat("P", 200*1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/files"):
			_, _ = fmt.Fprintf(w, `[
				{"id":1,"ddname":"JESYSMSG","stepname":"JES2","byte-count":%d},
				{"id":2,"ddname":"JESJCL","stepname":"JES2","byte-count":%d},
				{"id":3,"ddname":"SYSPRINT","stepname":"GO","byte-count":%d}
			]`, len(jesysmsg), len(jesjcl), len(sysprint))
		case strings.HasSuffix(r.URL.Path, "/files/1/records"):
			_, _ = w.Write([]byte(jesysmsg))
		case strings.HasSuffix(r.URL.Path, "/files/2/records"):
			_, _ = w.Write([]byte(jesjcl))
		case strings.HasSuffix(r.URL.Path, "/files/3/records"):
			_, _ = w.Write([]byte(sysprint))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	clients := newToolTestClients(t, srv.URL)

	// 1. run the tool with the default budget, forcing SYSPRINT into a link
	tool := NewGetAllSpoolsTool(clients, DefaultInlineMaxBytes)
	args, _ := json.Marshal(map[string]any{
		"system": "S0W1", "job_name": "MYJOB", "job_id": "JOB12345",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// 2. take the link URI out of the tool result
	var uri string
	var size int64
	for _, c := range result.Content {
		if c.Type == "resource_link" {
			uri, size = c.URI, c.Size
		}
	}
	if uri == "" {
		t.Fatal("no resource_link in the tool result")
	}
	if size != int64(len(sysprint)) {
		t.Errorf("link size = %d, want %d", size, len(sysprint))
	}

	// 3. resolve that exact URI through the resource registry
	registry := resources.NewResourceRegistry()
	if err := resources.RegisterAll(registry, clients, &config.Config{}); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	contents, err := registry.Read(context.Background(), uri)
	if err != nil {
		t.Fatalf("reading %q back as a resource: %v", uri, err)
	}
	if contents.Text != sysprint {
		t.Errorf("resource read returned %d bytes, want the full %d", len(contents.Text), len(sysprint))
	}
}

// TestUSSLinkIsReadableAsResource covers the same seam for USS paths, where
// URI encoding is in play: a path with characters that are unsafe in a URI
// must survive the round trip from link to read.
func TestUSSLinkIsReadableAsResource(t *testing.T) {
	const ussPath = "/u/ibmuser/notes #1 100% done.txt"
	body := strings.Repeat("U", 100*1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zosmf/restfiles/fs"+ussPath {
			t.Errorf("unexpected path: %q", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	clients := newToolTestClients(t, srv.URL)

	tool := NewReadUSSFileTool(clients, DefaultInlineMaxBytes)
	args, _ := json.Marshal(map[string]any{"system": "S0W1", "path": ussPath})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var uri string
	for _, c := range result.Content {
		if c.Type == "resource_link" {
			uri = c.URI
		}
	}
	if uri == "" {
		t.Fatal("no resource_link in the tool result")
	}
	if strings.ContainsAny(uri, " #") {
		t.Errorf("URI %q carries unencoded characters that a client would misread", uri)
	}

	registry := resources.NewResourceRegistry()
	if err := resources.RegisterAll(registry, clients, &config.Config{}); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	contents, err := registry.Read(context.Background(), uri)
	if err != nil {
		t.Fatalf("reading %q back as a resource: %v", uri, err)
	}
	if contents.Text != body {
		t.Errorf("resource read returned %d bytes, want the full %d", len(contents.Text), len(body))
	}
}
