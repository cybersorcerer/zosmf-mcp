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

// The success criterion of the MCP resources feature: small JES files
// inline, the large SYSPRINT as a link.
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

// A file the directory already reported as too large must become a link
// without being fetched: pulling a 2 GB SYSPRINT over the wire only to throw
// it away protects the model's context and nothing else.
func TestGetAllSpoolsSkipsFetchOfOversizedFile(t *testing.T) {
	jesysmsg := strings.Repeat("J", 2*1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/files"):
			_, _ = fmt.Fprintf(w, `[
				{"id":1,"ddname":"JESYSMSG","stepname":"JES2","byte-count":%d},
				{"id":2,"ddname":"SYSPRINT","stepname":"GO","byte-count":%d}
			]`, len(jesysmsg), 2*1024*1024*1024)
		case strings.HasSuffix(r.URL.Path, "/files/1/records"):
			_, _ = w.Write([]byte(jesysmsg))
		case strings.HasSuffix(r.URL.Path, "/files/2/records"):
			t.Error("the oversized file was fetched; its size was already known from the directory")
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

	var link *string
	var size int64
	for i, c := range result.Content {
		if c.Type == "resource_link" {
			link = &result.Content[i].URI
			size = c.Size
		}
	}
	if link == nil {
		t.Fatal("no resource_link for the oversized file")
	}
	if *link != "zosmf://S0W1/job/MYJOB/JOB12345/spool/2" {
		t.Errorf("link = %q", *link)
	}
	if size != 2*1024*1024*1024 {
		t.Errorf("link size = %d, want the size z/OSMF reported", size)
	}
}

// A file with no byte-count — z/OSMF omits it while a job is still running —
// must still be fetched, so the size can be measured.
func TestGetAllSpoolsFetchesWhenByteCountIsAbsent(t *testing.T) {
	small := strings.Repeat("J", 2*1024)
	big := strings.Repeat("P", 200*1024)
	fetched := map[string]bool{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/files"):
			_, _ = fmt.Fprint(w, `[
				{"id":1,"ddname":"JESYSMSG","stepname":"JES2"},
				{"id":2,"ddname":"SYSPRINT","stepname":"GO"}
			]`)
		case strings.HasSuffix(r.URL.Path, "/files/1/records"):
			fetched["1"] = true
			_, _ = w.Write([]byte(small))
		case strings.HasSuffix(r.URL.Path, "/files/2/records"):
			fetched["2"] = true
			_, _ = w.Write([]byte(big))
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
	if !fetched["1"] || !fetched["2"] {
		t.Errorf("without a byte-count both files must be fetched, got %v", fetched)
	}

	var links int
	for _, c := range result.Content {
		if c.Type == "resource_link" {
			links++
		}
	}
	if links != 1 {
		t.Errorf("links = %d, want 1 for the oversized file", links)
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
