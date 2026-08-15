package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
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

func TestDatasetTools_Annotations(t *testing.T) {
	reg := newTestRegistry(t)

	cases := []struct {
		name                                         string
		title                                        string
		readOnly, destructive, idempotent, openWorld bool
	}{
		{"zosmf_list_datasets", "List Datasets", true, false, true, false},
		{"zosmf_list_members", "List PDS Members", true, false, true, false},
		{"zosmf_read_dataset", "Read Dataset", true, false, true, false},
		{"zosmf_write_dataset", "Write Dataset", false, true, true, false},
		{"zosmf_create_dataset", "Allocate Dataset", false, false, false, false},
		{"zosmf_delete_dataset", "Delete Dataset", false, true, true, false},
		{"zosmf_acquire_dataset_enq", "Acquire Dataset ENQ", false, false, false, false},
		{"zosmf_release_dataset_enq", "Release Dataset ENQ", false, false, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := findTool(t, reg, tc.name)
			assertAnnotations(t, tool, tc.title, tc.readOnly, tc.destructive, tc.idempotent, tc.openWorld)
			if enum := systemEnum(t, tool, "system"); len(enum) != 2 {
				t.Errorf("system enum = %v, want 2 entries", enum)
			}
		})
	}
}

func TestCreateDataset_SaysItFailsOnExisting(t *testing.T) {
	tool := findTool(t, newTestRegistry(t), "zosmf_create_dataset")
	if !strings.Contains(tool.Description, "already exists") {
		t.Errorf("create_dataset must state that it fails on an existing dataset, got: %q", tool.Description)
	}
}

func TestFilesystemAndUSSTools_Annotations(t *testing.T) {
	reg := newTestRegistry(t)

	cases := []struct {
		name                                         string
		title                                        string
		readOnly, destructive, idempotent, openWorld bool
	}{
		{"zosmf_list_filesystems", "List Filesystems", true, false, true, false},
		{"zosmf_mount_filesystem", "Mount Filesystem", false, false, false, false},
		{"zosmf_unmount_filesystem", "Unmount Filesystem", false, true, true, false},
		{"zosmf_create_zfs", "Create zFS", false, false, false, false},
		{"zosmf_delete_zfs", "Delete zFS", false, true, true, false},
		{"zosmf_list_uss_files", "List USS Directory", true, false, true, false},
		{"zosmf_read_uss_file", "Read USS File", true, false, true, false},
		{"zosmf_write_uss_file", "Write USS File", false, true, true, false},
		{"zosmf_create_uss_file", "Create USS File or Directory", false, false, false, false},
		{"zosmf_delete_uss_file", "Delete USS File or Directory", false, true, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := findTool(t, reg, tc.name)
			assertAnnotations(t, tool, tc.title, tc.readOnly, tc.destructive, tc.idempotent, tc.openWorld)
			if enum := systemEnum(t, tool, "system"); len(enum) != 2 {
				t.Errorf("system enum = %v, want 2 entries", enum)
			}
		})
	}
}

// The listing tools were one-liners that never said what came back. A model
// had to call them to find out.
func TestListingTools_EnumerateTheirReturnFields(t *testing.T) {
	reg := newTestRegistry(t)

	cases := []struct {
		name   string
		fields []string
	}{
		{"zosmf_list_uss_files", []string{"size", "UID", "group", "symlink"}},
		{"zosmf_list_filesystems", []string{"mount point", "status", "device number"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc := findTool(t, reg, tc.name).Description
			for _, field := range tc.fields {
				if !strings.Contains(desc, field) {
					t.Errorf("description must name the %q it returns, got: %q", field, desc)
				}
			}
		})
	}
}

func TestConsoleTSOAndInventoryTools_Annotations(t *testing.T) {
	reg := newTestRegistry(t)

	cases := []struct {
		name                                         string
		title                                        string
		readOnly, destructive, idempotent, openWorld bool
	}{
		{"zosmf_issue_command", "Issue Operator Command", false, true, false, true},
		{"zosmf_get_hardcopy_log", "Read Hardcopy Log", true, false, true, false},
		{"zosmf_tso_command", "Execute TSO Command", false, true, false, true},
		{"zosmf_smpe_query_csi", "Query SMP/E CSI", true, false, true, false},
		{"zosmf_parmlib_validate", "Validate Parmlib Member", true, false, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := findTool(t, reg, tc.name)
			assertAnnotations(t, tool, tc.title, tc.readOnly, tc.destructive, tc.idempotent, tc.openWorld)
			if enum := systemEnum(t, tool, "system"); len(enum) != 2 {
				t.Errorf("system enum = %v, want 2 entries", enum)
			}
		})
	}
}

// Finding 6: these preconditions used to surface only as a runtime error.
func TestToolsDeclareTheirConfigPreconditions(t *testing.T) {
	reg := newTestRegistry(t)

	cases := []struct {
		name string
		keys []string
	}{
		{"zosmf_tso_command", []string{"tso_account", "tso_proc"}},
		{"zosmf_smpe_query_csi", []string{"csi_datasets", "target_zones"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc := findTool(t, reg, tc.name).Description
			for _, key := range tc.keys {
				if !strings.Contains(desc, key) {
					t.Errorf("description must name the required config key %q, got: %q", key, desc)
				}
			}
		})
	}
}

// The two tools that pass an arbitrary command through to the system must
// be flagged as open-world, or a client will treat them as bounded.
func TestArbitraryCommandTools_AreOpenWorld(t *testing.T) {
	reg := newTestRegistry(t)
	for _, name := range []string{"zosmf_issue_command", "zosmf_tso_command"} {
		tool := findTool(t, reg, name)
		if !tool.Annotations.OpenWorldHint {
			t.Errorf("%s: OpenWorldHint must be true", name)
		}
		if !strings.Contains(tool.Description, "WARNING") {
			t.Errorf("%s: description must carry a WARNING, got: %q", name, tool.Description)
		}
	}
}

func TestZOAUTools_UseTheirOwnNamespace(t *testing.T) {
	reg := newTestRegistry(t)

	names := make(map[string]bool)
	for _, tool := range reg.List() {
		names[tool.Name] = true
	}

	for _, old := range []string{
		"zosmf_zoau_pll", "zosmf_zoau_zinfo", "zosmf_zoau_llwhence", "zosmf_zoau_parmgrep",
		"zosmf_zoau_parmwhence", "zosmf_zoau_pproc", "zosmf_zoau_procgrep", "zosmf_zoau_procwhence",
		"zosmf_zoau_vtocls", "zosmf_zoau_version", "zosmf_zoau_exec",
	} {
		if names[old] {
			t.Errorf("legacy tool name %q is still registered — the rename is deliberate and has no alias", old)
		}
	}

	for _, want := range []string{
		"zoau_pll", "zoau_zinfo", "zoau_llwhence", "zoau_parmgrep", "zoau_parmwhence",
		"zoau_pproc", "zoau_procgrep", "zoau_procwhence", "zoau_vtocls", "zoau_version", "zoau_exec",
	} {
		if !names[want] {
			t.Errorf("tool %q is not registered", want)
		}
	}
}

func TestZOAUTools_AnnotationsAndSSHSystemEnum(t *testing.T) {
	reg := newTestRegistry(t)

	readOnly := []struct{ name, title string }{
		{"zoau_pll", "Show Linklist"},
		{"zoau_zinfo", "Show System Information"},
		{"zoau_llwhence", "Locate Linklist Member"},
		{"zoau_parmgrep", "Search Parmlib"},
		{"zoau_parmwhence", "Locate Parmlib Member"},
		{"zoau_pproc", "Show Proclib"},
		{"zoau_procgrep", "Search Proclib"},
		{"zoau_procwhence", "Locate Proclib Member"},
		{"zoau_vtocls", "List Volume VTOC"},
		{"zoau_version", "Show ZOAU Version"},
	}

	for _, tc := range readOnly {
		t.Run(tc.name, func(t *testing.T) {
			tool := findTool(t, reg, tc.name)
			assertAnnotations(t, tool, tc.title, true, false, true, false)

			enum := systemEnum(t, tool, "ssh_system")
			if len(enum) != 1 || enum[0] != "LPAR1" {
				t.Errorf("ssh_system enum = %v, want [LPAR1]", enum)
			}
		})
	}

	exec := findTool(t, reg, "zoau_exec")
	assertAnnotations(t, exec, "Execute USS Command", false, true, false, true)
	if enum := systemEnum(t, exec, "ssh_system"); len(enum) != 1 {
		t.Errorf("zoau_exec ssh_system enum = %v, want 1 entry", enum)
	}
	if !strings.Contains(exec.Description, "WARNING") {
		t.Errorf("zoau_exec must carry a WARNING, got: %q", exec.Description)
	}
	if !strings.Contains(exec.Description, "unrestricted") {
		t.Errorf("zoau_exec must say the access is unrestricted, got: %q", exec.Description)
	}
}

// No ZOAU tool may keep a 'system' parameter — that name belongs to the
// z/OSMF namespace and having both is what confused callers.
func TestZOAUTools_HaveNoSystemParameter(t *testing.T) {
	for _, tool := range newTestRegistry(t).List() {
		if !strings.HasPrefix(tool.Name, "zoau_") {
			continue
		}
		var schema struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("%s: %v", tool.Name, err)
		}
		if _, ok := schema.Properties["system"]; ok {
			t.Errorf("%s: still has a 'system' property, want 'ssh_system'", tool.Name)
		}
		if _, ok := schema.Properties["ssh_system"]; !ok {
			t.Errorf("%s: missing 'ssh_system' property", tool.Name)
		}
		found := false
		for _, r := range schema.Required {
			if r == "ssh_system" {
				found = true
			}
			if r == "system" {
				t.Errorf("%s: 'system' is still listed as required", tool.Name)
			}
		}
		if !found {
			t.Errorf("%s: 'ssh_system' must be required", tool.Name)
		}
	}
}

// Every registered tool must be fully described. This is the gate that
// catches a new tool added without annotations or without a system enum.
func TestAllTools_AreFullyDescribed(t *testing.T) {
	reg := newTestRegistry(t)
	tools := reg.List()

	if len(tools) != 45 {
		t.Errorf("registered %d tools, expected 45 — update this count deliberately when adding one", len(tools))
	}

	for _, tool := range tools {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.Annotations == nil {
				t.Fatal("Annotations is nil")
			}
			if tool.Annotations.Title == "" {
				t.Error("Annotations.Title is empty")
			}
			if tool.Annotations.ReadOnlyHint && tool.Annotations.DestructiveHint {
				t.Error("a read-only tool cannot be destructive")
			}

			if len(tool.Description) < 60 {
				t.Errorf("description is too thin (%d chars): %q", len(tool.Description), tool.Description)
			}

			var schema struct {
				Properties map[string]struct {
					Type        string   `json:"type"`
					Description string   `json:"description"`
					Enum        []string `json:"enum"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
				t.Fatalf("input schema is not valid JSON: %v", err)
			}

			for name, prop := range schema.Properties {
				if prop.Description == "" {
					t.Errorf("property %q has no description", name)
				}
			}

			// Exactly one of the two system parameters, and it must be enumerated.
			_, hasSystem := schema.Properties["system"]
			_, hasSSHSystem := schema.Properties["ssh_system"]
			switch {
			case hasSystem && hasSSHSystem:
				t.Error("has both 'system' and 'ssh_system'")
			case hasSystem:
				if len(schema.Properties["system"].Enum) == 0 {
					t.Error("'system' has no enum")
				}
			case hasSSHSystem:
				if len(schema.Properties["ssh_system"].Enum) == 0 {
					t.Error("'ssh_system' has no enum")
				}
			default:
				t.Error("has neither 'system' nor 'ssh_system'")
			}
		})
	}
}

// A server with no systems configured must still produce a valid schema —
// an empty enum would match nothing and make every call impossible.
func TestTools_OmitEnumWhenNothingIsConfigured(t *testing.T) {
	reg := mcp.NewToolRegistry()
	if err := RegisterAll(reg, zosmf.NewClientRegistry(), &config.Config{}, nil); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	for _, tool := range reg.List() {
		var schema struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("%s: %v", tool.Name, err)
		}
		if prop, ok := schema.Properties["system"]; ok && prop.Enum != nil {
			t.Errorf("%s: enum must be absent when no systems are configured, got %v", tool.Name, prop.Enum)
		}
	}
}
