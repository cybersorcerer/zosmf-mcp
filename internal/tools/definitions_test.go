package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/config"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

// newTestRegistry builds a fully populated tool registry against two z/OSMF
// systems and one SSH system, so schema enums have something to contain.
func newTestRegistry(t *testing.T) *mcp.ToolRegistry {
	t.Helper()

	cfg := &config.Config{
		Systems: []config.SystemConfig{
			{Name: "PROD", Host: "https://prod.invalid", TSOAccount: "ACCT", TSOProc: "IZUFPROC"},
			{Name: "TEST", Host: "https://test.invalid"},
		},
		SSHSystems: []config.SSHSystemConfig{
			{Name: "LPAR1", Host: "lpar1.invalid", Username: "user", Port: 22},
		},
	}

	clients := zosmf.NewClientRegistry()
	for _, sys := range cfg.Systems {
		client, err := zosmf.NewClient(sys)
		if err != nil {
			t.Fatalf("NewClient(%s): %v", sys.Name, err)
		}
		clients.Register(sys.Name, client)
	}
	sshReg := zosmf.NewSSHClientRegistry(cfg.SSHSystems)

	reg := mcp.NewToolRegistry()
	if err := RegisterAll(reg, clients, cfg, sshReg); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	return reg
}

func findTool(t *testing.T, reg *mcp.ToolRegistry, name string) mcp.Tool {
	t.Helper()
	for _, tool := range reg.List() {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q is not registered", name)
	return mcp.Tool{}
}

// systemEnum extracts properties.<param>.enum from a tool's input schema.
func systemEnum(t *testing.T, tool mcp.Tool, param string) []string {
	t.Helper()
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
		t.Fatalf("%s: input schema is not valid JSON: %v", tool.Name, err)
	}
	prop, ok := schema.Properties[param]
	if !ok {
		t.Fatalf("%s: input schema has no %q property", tool.Name, param)
	}
	return prop.Enum
}

func assertAnnotations(t *testing.T, tool mcp.Tool, wantTitle string, readOnly, destructive, idempotent, openWorld bool) {
	t.Helper()
	a := tool.Annotations
	if a == nil {
		t.Fatalf("%s: Annotations is nil", tool.Name)
	}
	if a.Title != wantTitle {
		t.Errorf("%s: Title = %q, want %q", tool.Name, a.Title, wantTitle)
	}
	if a.ReadOnlyHint != readOnly {
		t.Errorf("%s: ReadOnlyHint = %v, want %v", tool.Name, a.ReadOnlyHint, readOnly)
	}
	if a.DestructiveHint != destructive {
		t.Errorf("%s: DestructiveHint = %v, want %v", tool.Name, a.DestructiveHint, destructive)
	}
	if a.IdempotentHint != idempotent {
		t.Errorf("%s: IdempotentHint = %v, want %v", tool.Name, a.IdempotentHint, idempotent)
	}
	if a.OpenWorldHint != openWorld {
		t.Errorf("%s: OpenWorldHint = %v, want %v", tool.Name, a.OpenWorldHint, openWorld)
	}
}

func TestJobsAndAuthTools_Annotations(t *testing.T) {
	reg := newTestRegistry(t)

	cases := []struct {
		name                                         string
		title                                        string
		readOnly, destructive, idempotent, openWorld bool
	}{
		{"zosmf_list_jobs", "List Jobs", true, false, true, false},
		{"zosmf_get_job", "Get Job Details", true, false, true, false},
		{"zosmf_get_job_output", "List Job Spool Files", true, false, true, false},
		{"zosmf_get_spool_content", "Read Spool File", true, false, true, false},
		{"zosmf_get_all_spools", "Read All Spool Files", true, false, true, false},
		{"zosmf_submit_job", "Submit Job", false, false, false, false},
		{"zosmf_cancel_job", "Cancel Job", false, true, true, false},
		{"zosmf_purge_job", "Purge Job", false, true, true, false},
		{"zosmf_hold_job", "Hold Job", false, false, true, false},
		{"zosmf_release_job", "Release Job", false, false, true, false},
		{"zosmf_set_password", "Set System Password", false, false, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := findTool(t, reg, tc.name)
			assertAnnotations(t, tool, tc.title, tc.readOnly, tc.destructive, tc.idempotent, tc.openWorld)

			enum := systemEnum(t, tool, "system")
			if len(enum) != 2 || enum[0] != "PROD" || enum[1] != "TEST" {
				t.Errorf("system enum = %v, want [PROD TEST]", enum)
			}
		})
	}
}

func TestSubmitJob_DescribesThePollingFollowUp(t *testing.T) {
	tool := findTool(t, newTestRegistry(t), "zosmf_submit_job")
	if !strings.Contains(tool.Description, "zosmf_get_job") {
		t.Errorf("submit_job must point at the polling tool, got: %q", tool.Description)
	}
}

func TestDestructiveJobTools_WarnInProse(t *testing.T) {
	reg := newTestRegistry(t)
	for _, name := range []string{"zosmf_cancel_job", "zosmf_purge_job"} {
		tool := findTool(t, reg, name)
		if !strings.Contains(tool.Description, "WARNING") {
			t.Errorf("%s: description must carry a WARNING, got: %q", name, tool.Description)
		}
	}
}
