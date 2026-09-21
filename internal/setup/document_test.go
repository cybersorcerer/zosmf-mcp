package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("testdata/shared.yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "shared.yaml")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The file belongs to four tools. Updating our keys must not disturb theirs,
// their comments, or keys we have never heard of — compared as bytes
// against a golden file, not by spot-checking a handful of substrings: the
// fixture carries a blank line between its two top-level entries and an
// aligned inline comment, both of which a substring check would not notice
// going missing.
func TestUpsertKeepsForeignContent(t *testing.T) {
	path := fixture(t)
	doc, err := LoadDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.UpsertSystem("MAINT", []Entry{
		{Key: "csi", Value: []string{"NEW.SMPE.GLOBAL.CSI"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(path); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/shared.upserted.golden.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(want) {
		t.Errorf("result does not match the golden file:\nwant:\n%s\ngot:\n%s", want, out)
	}
}

func TestUpsertAppendsUnknownSystem(t *testing.T) {
	path := fixture(t)
	doc, _ := LoadDocument(path)
	if _, err := doc.UpsertSystem("PROD", []Entry{
		{Key: "host", Value: "https://zos2.example.com:443"},
		{Key: "user", Value: "IBMUSER"},
	}); err != nil {
		t.Fatal(err)
	}
	out, _ := doc.Bytes()
	text := string(out)
	if strings.Count(text, "name: MAINT") != 1 || !strings.Contains(text, "name: PROD") {
		t.Errorf("expected MAINT kept and PROD added:\n%s", text)
	}
}

// The server uppercases system names, so the file must not end up with two
// entries that differ only in case.
func TestUpsertMatchesNameCaseInsensitively(t *testing.T) {
	path := fixture(t)
	doc, _ := LoadDocument(path)
	if _, err := doc.UpsertSystem("maint", []Entry{{Key: "user", Value: "OTHER"}}); err != nil {
		t.Fatal(err)
	}
	out, _ := doc.Bytes()
	text := string(out)
	if n := strings.Count(text, "- name:"); n != 1 {
		t.Errorf("got %d systems, want 1:\n%s", n, out)
	}
	if !strings.Contains(text, "# do not touch, ticket 123") {
		t.Errorf("result lost the comment on user after re-setting it:\n%s", text)
	}
}

func TestUpsertIsIdempotent(t *testing.T) {
	path := fixture(t)
	doc, _ := LoadDocument(path)
	entries := []Entry{{Key: "user", Value: "IBMUSER"}}

	first, err := doc.UpsertSystem("MAINT", entries)
	if err != nil {
		t.Fatal(err)
	}
	second, err := doc.UpsertSystem("MAINT", entries)
	if err != nil {
		t.Fatal(err)
	}
	if first {
		t.Error("writing the value it already has reported a change")
	}
	if second {
		t.Error("the second identical upsert reported a change")
	}
}

func TestLoadDocumentOfMissingFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.yaml")
	doc, err := LoadDocument(path)
	if err != nil {
		t.Fatalf("a missing file must not be an error: %v", err)
	}
	if _, err := doc.UpsertSystem("MAINT", []Entry{{Key: "user", Value: "IBMUSER"}}); err != nil {
		t.Fatal(err)
	}
	out, _ := doc.Bytes()
	if !strings.Contains(string(out), "servers:") {
		t.Errorf("a new file must use the servers: spelling:\n%s", out)
	}
}

func TestEnsureDefaultTransportAddsWhenAbsent(t *testing.T) {
	path := fixture(t)
	doc, _ := LoadDocument(path)
	changed, err := doc.EnsureDefaultTransport()
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("expected a change: the fixture carries no transport key")
	}
	out, _ := doc.Bytes()
	text := string(out)
	if !strings.Contains(text, "transport:") || !strings.Contains(text, "stdio: true") {
		t.Errorf("expected transport: {stdio: true} to be added:\n%s", text)
	}
}

func TestEnsureDefaultTransportLeavesExistingAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "http.yaml")
	src := "servers:\n  - name: MAINT\n    host: https://zos1.example.com:443\n" +
		"    user: IBMUSER\ntransport:\n  streamable_http:\n    enabled: true\n    listen: \":8081\"\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := doc.EnsureDefaultTransport()
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("an existing transport block must not be reported as changed")
	}
	out, _ := doc.Bytes()
	if string(out) != src {
		t.Errorf("existing transport block was altered:\nwant:\n%s\ngot:\n%s", src, out)
	}
}

func TestEnsureDefaultTransportIsIdempotent(t *testing.T) {
	path := fixture(t)
	doc, _ := LoadDocument(path)
	first, err := doc.EnsureDefaultTransport()
	if err != nil {
		t.Fatal(err)
	}
	if !first {
		t.Fatal("first call should add the block")
	}
	second, err := doc.EnsureDefaultTransport()
	if err != nil {
		t.Fatal(err)
	}
	if second {
		t.Error("second call should report no change")
	}
	out, _ := doc.Bytes()
	if n := strings.Count(string(out), "transport:"); n != 1 {
		t.Errorf("expected exactly one transport: key, got %d:\n%s", n, out)
	}
}

func TestSaveLeavesABackup(t *testing.T) {
	path := fixture(t)
	doc, _ := LoadDocument(path)
	if _, err := doc.UpsertSystem("MAINT", []Entry{{Key: "user", Value: "OTHER"}}); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("no backup next to the file: %v", err)
	}
}

// A file whose top level is not a mapping — a bare scalar, here — has
// nowhere for UpsertSystem to put a servers: key. Before this guard,
// upsert() appended onto the scalar node's Content anyway: the encoder
// ignores that on a scalar, so the write silently did nothing while still
// reporting changed=true and no error.
func TestUpsertOnNonMappingRootFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scalar.yaml")
	if err := os.WriteFile(path, []byte("just a string\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := doc.UpsertSystem("MAINT", []Entry{{Key: "user", Value: "IBMUSER"}})
	if err == nil {
		t.Fatal("expected an error, the top level is not a mapping")
	}
	if changed {
		t.Error("a failed upsert must not report a change")
	}
}

// yaml.Unmarshal keeps only the first of several `---` separated
// documents; loading such a file and writing it back would silently drop
// everything after the first one.
func TestLoadDocumentRejectsMultipleDocuments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.yaml")
	src := "servers:\n  - name: MAINT\n    user: IBMUSER\n---\nother: doc\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDocument(path); err == nil {
		t.Fatal("expected an error for a file with more than one YAML document")
	}
}
