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
