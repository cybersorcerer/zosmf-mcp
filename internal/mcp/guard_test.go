package mcp

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeTextLeavesCleanTextAlone(t *testing.T) {
	for _, s := range []string{"", "IEF142I MYJOB GO - STEP WAS EXECUTED - COND CODE 0000\n", "Größe\tÄÖÜ ß €\r\n"} {
		if got := SanitizeText(s); got != s {
			t.Errorf("SanitizeText(%q) = %q, want unchanged", s, got)
		}
	}
}

func TestSanitizeTextReplacesProblemCharacters(t *testing.T) {
	in := "A\x00B\x1bC" + string(rune(0x85)) + "D" + string(rune(0x90)) + "E" + string(rune(0x2028)) + "F" + string(rune(0xFEFF)) + "G\x7fH" + string([]byte{0xff, 0xfe}) + "I"
	got := SanitizeText(in)
	want := "A.B.C\nD.E.F.G.H" + string(utf8.RuneError) + "I"
	if got != want {
		t.Fatalf("SanitizeText = %q, want %q", got, want)
	}
	if !utf8.ValidString(got) {
		t.Fatal("result is not valid UTF-8")
	}
}

func TestCutTextNeverSplitsARune(t *testing.T) {
	s := strings.Repeat("ä", 100) // 2 bytes each
	for max := 1; max < 50; max++ {
		got := cutText(s, max)
		if !utf8.ValidString(got) || len(got) > max {
			t.Fatalf("cutText(max=%d) = %q (len %d)", max, got, len(got))
		}
	}
}

func TestCutTextPrefersLineBoundary(t *testing.T) {
	s := strings.Repeat("x", 70) + "\n" + strings.Repeat("y", 70)
	if got := cutText(s, 100); got != strings.Repeat("x", 70)+"\n" {
		t.Fatalf("cutText = %q", got)
	}
}

func TestGuardToolResultCapsTotalAndKeepsLinks(t *testing.T) {
	res := &ToolCallResult{Content: []Content{
		TextContent(strings.Repeat("a\n", 600)),
		TextContent(strings.Repeat("b\n", 600)),
		ResourceLink("zosmf://S/spool/J/1", "SYSPRINT", "d", "text/plain", 1),
	}}
	if !GuardToolResult(res, 1000) {
		t.Fatal("expected truncation")
	}
	text, links := 0, 0
	for _, c := range res.Content {
		if c.Type == "resource_link" {
			links++
			continue
		}
		if !strings.Contains(c.Text, "zosmf-mcp: output truncated") {
			text += len(c.Text)
		}
	}
	if text > 1000 {
		t.Errorf("inline text %d bytes exceeds cap", text)
	}
	if links != 1 {
		t.Errorf("resource_link dropped: %d links", links)
	}
	last := res.Content[len(res.Content)-1]
	if !strings.Contains(last.Text, "of 2400 bytes shown") {
		t.Errorf("missing truncation note: %q", last.Text)
	}
}

func TestGuardToolResultSmallIsUntouched(t *testing.T) {
	res := &ToolCallResult{Content: []Content{TextContent("ok\n")}}
	if GuardToolResult(res, 1000) || res.Content[0].Text != "ok\n" || len(res.Content) != 1 {
		t.Fatalf("small result changed: %+v", res)
	}
}

func TestGuardResultMarshalsToValidJSON(t *testing.T) {
	res := &ToolCallResult{Content: []Content{TextContent(string([]byte{0xc3}) + string(rune(0x2028)) + "\x00" + strings.Repeat("ü", 50000))}}
	GuardToolResult(res, MaxResultTextBytes)
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var back ToolCallResult
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("round trip: %v", err)
	}
}

func TestGuardResourceContents(t *testing.T) {
	big := &ResourceContents{URI: "u", Text: strings.Repeat("z\n", MaxResourceTextBytes)}
	if !GuardResourceContents(big) || len(big.Text) > MaxResourceTextBytes+512 {
		t.Fatalf("text not capped: %d", len(big.Text))
	}

	bin := &ResourceContents{URI: "zosmf://S/ds/LOAD(X)", Blob: base64.StdEncoding.EncodeToString(make([]byte, 64*1024))}
	if !GuardResourceContents(bin) || bin.Blob != "" || !strings.Contains(bin.Text, "binary") {
		t.Fatalf("large blob not replaced: %+v", bin.Text)
	}

	small := &ResourceContents{URI: "u", Blob: "AAAA"}
	if GuardResourceContents(small) || small.Blob != "AAAA" {
		t.Fatal("small blob changed")
	}
}
