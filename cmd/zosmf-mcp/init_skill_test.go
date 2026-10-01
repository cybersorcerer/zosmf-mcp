package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The skill must install from the binary alone: a user who downloaded a
// release has no repository to copy it from.
func TestInitSkillInstallsFromTheBinary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cases := []struct{ agent, scope, dir string }{
		{"claude", "user", filepath.Join(home, ".claude", "skills", "zosmf-mcp-setup")},
		{"opencode", "user", filepath.Join(home, ".config", "opencode", "skills", "zosmf-mcp-setup")},
		{"pi", "user", filepath.Join(home, ".pi", "agent", "skills", "zosmf-mcp-setup")},
		{"claude", "project", filepath.Join(".claude", "skills", "zosmf-mcp-setup")},
	}
	t.Chdir(t.TempDir())
	for _, c := range cases {
		code, out, errOut := run(t, false, "skill", "--agent", c.agent, "--scope", c.scope)
		if code != 0 {
			t.Fatalf("%s/%s: exit %d, stderr: %s", c.agent, c.scope, code, errOut)
		}
		wrapper, err := os.ReadFile(filepath.Join(c.dir, "SKILL.md"))
		if err != nil || !strings.Contains(string(wrapper), "name: zosmf-mcp-setup") {
			t.Errorf("%s/%s: no wrapper in %s: %v", c.agent, c.scope, c.dir, err)
		}
		procedure, err := os.ReadFile(filepath.Join(c.dir, "setup.md"))
		if err != nil || !strings.Contains(string(procedure), "init restore") {
			t.Errorf("%s/%s: no setup.md in %s: %v", c.agent, c.scope, c.dir, err)
		}
		if !strings.Contains(out, c.dir) {
			t.Errorf("%s/%s: output does not name %s:\n%s", c.agent, c.scope, c.dir, out)
		}
	}
}

func TestInitSkillRejectsAnUnknownAgent(t *testing.T) {
	t.Chdir(t.TempDir())
	if code, _, _ := run(t, false, "skill", "--agent", "vim"); code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
}
