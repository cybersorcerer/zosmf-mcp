package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The success criterion from docs/2026-08-17-mcp-resources-design.md:
// small JES files inline, the large SYSPRINT as a link.
func TestGetAllSpoolsBudget(t *testing.T) {
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

	tool := NewGetAllSpoolsTool(newToolTestClients(t, srv.URL), DefaultInlineMaxBytes)
	args, _ := json.Marshal(map[string]any{
		"system": "S0W1", "job_name": "MYJOB", "job_id": "JOB12345",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var inlineText string
	var links []string
	for _, c := range result.Content {
		switch c.Type {
		case "text":
			inlineText += c.Text
		case "resource_link":
			links = append(links, c.URI)
		}
	}

	if !strings.Contains(inlineText, jesysmsg) {
		t.Error("JESYSMSG is not inline, want full content")
	}
	if !strings.Contains(inlineText, jesjcl) {
		t.Error("JESJCL is not inline, want full content")
	}
	if strings.Contains(inlineText, sysprint) {
		t.Error("SYSPRINT is inline, want a link")
	}

	if len(links) != 1 {
		t.Fatalf("len(links) = %d, want 1", len(links))
	}
	if links[0] != "zosmf://S0W1/job/MYJOB/JOB12345/spool/3" {
		t.Errorf("link = %q", links[0])
	}
}

func TestGetAllSpoolsFirstFileOversized(t *testing.T) {
	huge := strings.Repeat("P", 200*1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/files"):
			_, _ = fmt.Fprintf(w, `[{"id":1,"ddname":"SYSPRINT","stepname":"GO","byte-count":%d}]`, len(huge))
		case strings.HasSuffix(r.URL.Path, "/files/1/records"):
			_, _ = w.Write([]byte(huge))
		}
	}))
	defer srv.Close()

	tool := NewGetAllSpoolsTool(newToolTestClients(t, srv.URL), DefaultInlineMaxBytes)
	args, _ := json.Marshal(map[string]any{
		"system": "S0W1", "job_name": "MYJOB", "job_id": "JOB12345",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var sawText, sawLink bool
	for _, c := range result.Content {
		if c.Type == "text" && len(c.Text) > 0 {
			sawText = true
		}
		if c.Type == "resource_link" {
			sawLink = true
		}
	}
	if !sawText {
		t.Error("no readable text — a result must never be link-only")
	}
	if !sawLink {
		t.Error("no link for the oversized file")
	}
}
