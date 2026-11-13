package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetSpoolContentSearchIsServerSide(t *testing.T) {
	var gotQuery, gotRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/files/3/records") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		gotRange = r.Header.Get("X-IBM-Record-Range")
		w.Header().Set("X-IBM-Record-Range", "1200,2")
		_, _ = w.Write([]byte("IEW2456E SYMBOL X UNRESOLVED\nIEW2008I DONE\n"))
	}))
	defer srv.Close()

	tool := NewGetSpoolContentTool(newToolTestClients(t, srv.URL), DefaultInlineMaxBytes)
	args, _ := json.Marshal(map[string]any{
		"system": "S0W1", "job_name": "MYJOB", "job_id": "JOB12345", "file_id": 3,
		"research": "IEW2456E|ABEND", "max_records": "20", "case_sensitive": true, "record_range": "1000-0",
	})
	res, err := tool.Execute(context.Background(), args)
	if err != nil || res.IsError {
		t.Fatalf("Execute: %v %+v", err, res)
	}
	for _, want := range []string{"research=IEW2456E%7CABEND", "maxreturnsize=20", "insensitive=false"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q lacks %q", gotQuery, want)
		}
	}
	if gotRange != "1000-0" {
		t.Errorf("X-IBM-Record-Range = %q", gotRange)
	}
	var out struct {
		Content    string `json:"content"`
		Found      bool   `json:"found"`
		FirstMatch int    `json:"first_match"`
		NumRecords int    `json:"num_records"`
	}
	if err := json.Unmarshal([]byte(res.Content[0].Text), &out); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if !out.Found || out.FirstMatch != 1200 || out.NumRecords != 2 || !strings.Contains(out.Content, "IEW2456E") {
		t.Errorf("unexpected result: %+v", out)
	}
}

func TestGetSpoolContentRejectsBadArguments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request expected, got %s", r.URL)
	}))
	defer srv.Close()
	tool := NewGetSpoolContentTool(newToolTestClients(t, srv.URL), DefaultInlineMaxBytes)

	for name, extra := range map[string]map[string]any{
		"both searches": {"search": "A", "research": "B"},
		"bad range":     {"record_range": "10-"},
		"range letters": {"record_range": "a,5"},
	} {
		a := map[string]any{"system": "S0W1", "job_name": "J", "job_id": "JOB1", "file_id": "2"}
		for k, v := range extra {
			a[k] = v
		}
		args, _ := json.Marshal(a)
		res, _ := tool.Execute(context.Background(), args)
		if !res.IsError {
			t.Errorf("%s: expected error, got %+v", name, res)
		}
	}
}
