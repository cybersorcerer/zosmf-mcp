package mcp

import (
	"strings"
	"testing"
)

func TestFormatArgsRedactsSecrets(t *testing.T) {
	out := formatArgs([]byte(`{"system":"S0W1","password":"hunter2","new_passphrase":"abc"}`))
	if strings.Contains(out, "hunter2") || strings.Contains(out, "abc") {
		t.Fatalf("secret leaked into log output: %s", out)
	}
	if !strings.Contains(out, "S0W1") {
		t.Fatalf("non-sensitive value missing: %s", out)
	}
}

func TestFormatArgsTruncates(t *testing.T) {
	long := `{"data":"` + strings.Repeat("x", 2000) + `"}`
	out := formatArgs([]byte(long))
	if len(out) > maxLoggedArgsLen+20 {
		t.Fatalf("output not truncated: %d bytes", len(out))
	}
}

func TestFormatArgsEmpty(t *testing.T) {
	if got := formatArgs(nil); got != "{}" {
		t.Fatalf("got %q, want {}", got)
	}
}
