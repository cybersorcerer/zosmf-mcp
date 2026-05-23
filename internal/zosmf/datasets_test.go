package zosmf

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient returns a Client whose baseURL points at the given httptest
// server. Auth is left empty so Do() will skip BasicAuth.
func newTestClient(server *httptest.Server) *Client {
	return &Client{
		httpClient: server.Client(),
		baseURL:    server.URL,
		systemName: "TESTSYS",
	}
}

func TestReadDataset_NoSearch(t *testing.T) {
	var capturedPath string
	var capturedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.RequestURI()
		capturedHeader = r.Header.Get("X-IBM-Data-Type")
		_, _ = w.Write([]byte("line1\nline2\n"))
	}))
	defer srv.Close()
	c := newTestClient(srv)

	content, err := c.ReadDataset("HLQ.SEQ", "")
	if err != nil {
		t.Fatalf("ReadDataset: %v", err)
	}
	if content != "line1\nline2\n" {
		t.Errorf("content = %q", content)
	}
	if strings.Contains(capturedPath, "search=") {
		t.Errorf("unexpected search query: %s", capturedPath)
	}
	if capturedHeader != "text" {
		t.Errorf("X-IBM-Data-Type = %q, want text", capturedHeader)
	}
}

func TestReadDataset_WithEncoding(t *testing.T) {
	var capturedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get("X-IBM-Data-Type")
		_, _ = w.Write([]byte("data"))
	}))
	defer srv.Close()
	c := newTestClient(srv)

	if _, err := c.ReadDataset("HLQ.SEQ", "IBM-1141"); err != nil {
		t.Fatal(err)
	}
	if capturedHeader != "text;fileEncoding=IBM-1141" {
		t.Errorf("X-IBM-Data-Type = %q", capturedHeader)
	}
}

func TestReadDatasetEx_SearchInsensitiveDefault(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.RequestURI()
		w.Header().Set("X-IBM-Record-Range", "5,3")
		_, _ = w.Write([]byte("matching-record-1\nmatching-record-2\nmatching-record-3\n"))
	}))
	defer srv.Close()
	c := newTestClient(srv)

	res, err := c.ReadDatasetEx("HLQ.PDS(MEM)", "", "IKJEFT01", true, 0)
	if err != nil {
		t.Fatalf("ReadDatasetEx: %v", err)
	}
	if !strings.Contains(capturedPath, "search=IKJEFT01") {
		t.Errorf("missing search query: %s", capturedPath)
	}
	if strings.Contains(capturedPath, "insensitive=") {
		t.Errorf("insensitive should be omitted when true (default): %s", capturedPath)
	}
	if !res.Found {
		t.Error("expected Found=true with X-IBM-Record-Range=5,3")
	}
	if res.FirstMatch != 5 {
		t.Errorf("FirstMatch = %d, want 5", res.FirstMatch)
	}
	if res.NumRecords != 3 {
		t.Errorf("NumRecords = %d, want 3", res.NumRecords)
	}
}

func TestReadDatasetEx_SearchCaseSensitive(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.RequestURI()
		w.Header().Set("X-IBM-Record-Range", "1,1")
		_, _ = w.Write([]byte("hit\n"))
	}))
	defer srv.Close()
	c := newTestClient(srv)

	if _, err := c.ReadDatasetEx("HLQ.SEQ", "", "Foo", false, 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(capturedPath, "insensitive=false") {
		t.Errorf("missing insensitive=false: %s", capturedPath)
	}
}

func TestReadDatasetEx_MaxReturnSize(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.RequestURI()
		w.Header().Set("X-IBM-Record-Range", "1,10")
		_, _ = w.Write([]byte("rows"))
	}))
	defer srv.Close()
	c := newTestClient(srv)

	if _, err := c.ReadDatasetEx("HLQ.SEQ", "", "x", true, 10); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(capturedPath, "maxreturnsize=10") {
		t.Errorf("missing maxreturnsize=10: %s", capturedPath)
	}
}

func TestReadDatasetEx_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-IBM-Record-Range", "0,0")
		// z/OSMF returns 200 with empty body when no match in search mode
	}))
	defer srv.Close()
	c := newTestClient(srv)

	res, err := c.ReadDatasetEx("HLQ.SEQ", "", "nomatch", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Found {
		t.Error("expected Found=false for X-IBM-Record-Range=0,0")
	}
	if res.NumRecords != 0 {
		t.Errorf("NumRecords = %d, want 0", res.NumRecords)
	}
}

func TestReadDatasetEx_PDSMemberPathEscape(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c := newTestClient(srv)

	if _, err := c.ReadDataset("SYS1.PARMLIB(IEASYS00)", ""); err != nil {
		t.Fatal(err)
	}
	// parentheses around the member must NOT be percent-encoded for z/OSMF
	if !strings.HasSuffix(capturedPath, "/SYS1.PARMLIB(IEASYS00)") {
		t.Errorf("path = %q — member parentheses must not be escaped", capturedPath)
	}
}
