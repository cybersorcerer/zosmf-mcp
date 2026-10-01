package main

import "testing"

func TestVersionInfoLayout(t *testing.T) {
	got := versionInfo("v0.6.0", "9aa0752")
	want := "zosmf-mcp 0.6.0\n" +
		"Commit: 9aa0752\n" +
		"Copyright (c) 2026 Ronny Funk (Cybersorcerer)\n"
	if got != want {
		t.Errorf("versionInfo() =\n%q\nwant\n%q", got, want)
	}
}

// git describe prefixes the tag, the printed version does not.
func TestVersionInfoDropsTheTagPrefix(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"v0.6.0", "zosmf-mcp 0.6.0\n"},
		{"v0.6.0-12-gabc1234", "zosmf-mcp 0.6.0-12-gabc1234\n"},
		{"dev", "zosmf-mcp dev\n"},
	} {
		got := versionInfo(tc.in, "abc1234")
		if first := got[:len(tc.want)]; first != tc.want {
			t.Errorf("versionInfo(%q) starts with %q, want %q", tc.in, first, tc.want)
		}
	}
}

// --version and serverInfo report the same string, so the tag prefix is
// stripped in one place both of them use.
func TestDisplayVersionStripsTheTagPrefix(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"v0.6.0", "0.6.0"},
		{"v0.6.0-1-g9f942ac", "0.6.0-1-g9f942ac"},
		{"dev", "dev"},
		{"", ""},
	} {
		if got := displayVersion(tc.in); got != tc.want {
			t.Errorf("displayVersion(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
