package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

// --- List Datasets ---

type listDatasetsTool struct {
	clients *zosmf.ClientRegistry
}

type listDatasetsArgs struct {
	System  string `json:"system"`
	DSLevel string `json:"dslevel"`
	VolSer  string `json:"volser,omitempty"`
	Start   string `json:"start,omitempty"`
}

func NewListDatasetsTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &listDatasetsTool{clients: clients}
}

func (t *listDatasetsTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_list_datasets",
		Description: "List z/OS datasets matching a pattern. Use standard z/OS dataset level patterns " +
			"(e.g. 'SYS1.PARM*', 'USER.**.JCL'). Returns dataset attributes: DSORG, RECFM, LRECL, BLKSIZE, " +
			"volume serial, creation/reference/expiry dates, used space, migration status, and dataset type (PDS, PDSE, etc.).",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{
					"type":        "string",
					"description": "Name of the z/OS system",
				},
				"dslevel": map[string]any{
					"type":        "string",
					"description": "Dataset name pattern (e.g. 'SYS1.PARM*', 'USER.**'). Supports * and ** wildcards.",
				},
				"volser": map[string]any{
					"type":        "string",
					"description": "Optional volume serial (1-6 chars, no wildcards). If specified, searches uncatalogued datasets on that volume matching the dslevel pattern.",
				},
				"start": map[string]any{
					"type":        "string",
					"description": "Optional first dataset name to return (max 44 chars, no wildcards). Useful for pagination.",
				},
			},
			"required": []string{"system", "dslevel"},
		}),
	}
}

func (t *listDatasetsTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args listDatasetsArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	datasets, err := client.ListDatasets(args.DSLevel, args.VolSer, args.Start)
	if err != nil {
		return mcp.ErrorResult("failed to list datasets: " + err.Error()), nil
	}

	data, _ := json.MarshalIndent(datasets, "", "  ")
	return mcp.TextResult(string(data)), nil
}

// --- List Members ---

type listMembersTool struct {
	clients *zosmf.ClientRegistry
}

type listMembersArgs struct {
	System  string `json:"system"`
	Dataset string `json:"dataset"`
}

func NewListMembersTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &listMembersTool{clients: clients}
}

func (t *listMembersTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_list_members",
		Description: "List members of a PDS or PDSE dataset on z/OS. Returns member statistics: " +
			"name, version, modification count, creation/modification date and time, " +
			"current/initial/modified line counts, last modifying user, and SCLM indicator.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{
					"type":        "string",
					"description": "Name of the z/OS system",
				},
				"dataset": map[string]any{
					"type":        "string",
					"description": "PDS/PDSE dataset name (e.g. 'SYS1.PARMLIB')",
				},
			},
			"required": []string{"system", "dataset"},
		}),
	}
}

func (t *listMembersTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args listMembersArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	members, err := client.ListMembers(args.Dataset)
	if err != nil {
		return mcp.ErrorResult("failed to list members: " + err.Error()), nil
	}

	data, _ := json.MarshalIndent(members, "", "  ")
	return mcp.TextResult(string(data)), nil
}

// --- Read Dataset ---

type readDatasetTool struct {
	clients *zosmf.ClientRegistry
}

type readDatasetArgs struct {
	System   string `json:"system"`
	Dataset  string `json:"dataset"`
	Encoding string `json:"encoding,omitempty"`
}

func NewReadDatasetTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &readDatasetTool{clients: clients}
}

func (t *readDatasetTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_read_dataset",
		Description: "Read the content of a z/OS sequential dataset or PDS/PDSE member. " +
			"For a PDS member, use the format 'DATASET.NAME(MEMBER)'.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{
					"type":        "string",
					"description": "Name of the z/OS system",
				},
				"dataset": map[string]any{
					"type":        "string",
					"description": "Dataset name, e.g. 'SYS1.PARMLIB(IEASYS00)' or 'USER.DATA.SEQ'",
				},
				"encoding": map[string]any{
					"type":        "string",
					"description": "Optional EBCDIC code page for z/OSMF conversion, e.g. 'IBM-1141'. Omit to use z/OSMF default (IBM-1047).",
				},
			},
			"required": []string{"system", "dataset"},
		}),
	}
}

func (t *readDatasetTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args readDatasetArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	content, err := client.ReadDataset(args.Dataset, args.Encoding)
	if err != nil {
		return mcp.ErrorResult("failed to read dataset: " + err.Error()), nil
	}

	return mcp.TextResult(content), nil
}

// --- Write Dataset ---

type writeDatasetTool struct {
	clients *zosmf.ClientRegistry
}

type writeDatasetArgs struct {
	System     string `json:"system"`
	Dataset    string `json:"dataset"`
	Content    string `json:"content"`
	SessionRef string `json:"session_ref"`
	Encoding   string `json:"encoding,omitempty"`
	ReleaseEnq bool   `json:"release_enq,omitempty"`
}

func NewWriteDatasetTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &writeDatasetTool{clients: clients}
}

func (t *writeDatasetTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_write_dataset",
		Description: "Write content to a z/OS sequential dataset or PDS/PDSE member. " +
			"WARNING: This overwrites the existing content. " +
			"For a PDS member, use the format 'DATASET.NAME(MEMBER)'.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{
					"type":        "string",
					"description": "Name of the z/OS system",
				},
				"dataset": map[string]any{
					"type":        "string",
					"description": "Dataset name, e.g. 'USER.JCL(MYJOB)' or 'USER.DATA.SEQ'",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Content to write to the dataset",
				},
				"session_ref": map[string]any{
					"type":        "string",
					"description": "Session reference from zosmf_acquire_dataset_enq (required when ENQ is held)",
				},
				"encoding": map[string]any{
					"type":        "string",
					"description": "Optional EBCDIC code page for z/OSMF conversion, e.g. 'IBM-1141'. Omit to use z/OSMF default (IBM-1047).",
				},
				"release_enq": map[string]any{
					"type":        "boolean",
					"description": "If true, sends X-IBM-Release-ENQ: true to release the ENQ after writing. Default false — keep ENQ held for subsequent saves.",
				},
			},
			"required": []string{"system", "dataset", "content"},
		}),
	}
}

func (t *writeDatasetTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args writeDatasetArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	if err := client.WriteDataset(args.Dataset, args.Content, args.SessionRef, args.Encoding, args.ReleaseEnq); err != nil {
		return mcp.ErrorResult("failed to write dataset: " + err.Error()), nil
	}

	return mcp.TextResult(fmt.Sprintf("Successfully wrote to %s", args.Dataset)), nil
}

// --- Create Dataset ---

type createDatasetTool struct {
	clients *zosmf.ClientRegistry
}

type createDatasetArgs struct {
	System    string `json:"system"`
	Dataset   string `json:"dataset"`
	DSOrg     string `json:"dsorg"`
	AlcUnit   string `json:"alcunit"`
	Primary   int    `json:"primary"`
	Secondary int    `json:"secondary"`
	RecFm     string `json:"recfm"`
	LRecL     int    `json:"lrecl"`
	BlkSize   int    `json:"blksize,omitempty"`
	DirBlk    int    `json:"dirblk,omitempty"`
	VolSer    string `json:"volser,omitempty"`
	Unit      string `json:"unit,omitempty"`
	StorClass string `json:"storclass,omitempty"`
	MgmtClass string `json:"mgmtclass,omitempty"`
	DataClass string `json:"dataclass,omitempty"`
	DSNType   string `json:"dsntype,omitempty"`
}

func NewCreateDatasetTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &createDatasetTool{clients: clients}
}

func (t *createDatasetTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_create_dataset",
		Description: "Allocate a new z/OS dataset (sequential PS or partitioned PDS/PDSE).",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":    map[string]any{"type": "string", "description": "Name of the z/OS system"},
				"dataset":   map[string]any{"type": "string", "description": "Dataset name to create (e.g. 'USER.NEW.DATA')"},
				"dsorg":     map[string]any{"type": "string", "description": "Dataset organization: 'PS' (sequential) or 'PO' (partitioned)", "enum": []string{"PS", "PO"}},
				"alcunit":   map[string]any{"type": "string", "description": "Allocation unit: 'TRK' or 'CYL'", "enum": []string{"TRK", "CYL"}},
				"primary":   map[string]any{"type": "integer", "description": "Primary space allocation"},
				"secondary": map[string]any{"type": "integer", "description": "Secondary space allocation"},
				"recfm":     map[string]any{"type": "string", "description": "Record format: 'FB', 'VB', 'F', 'V', 'U'"},
				"lrecl":     map[string]any{"type": "integer", "description": "Logical record length (e.g. 80, 132, 0 for U)"},
				"blksize":   map[string]any{"type": "integer", "description": "Block size (optional, system will calculate if omitted)"},
				"dirblk":    map[string]any{"type": "integer", "description": "Number of directory blocks (required for PO datasets)"},
				"volser":    map[string]any{"type": "string", "description": "Volume serial (optional)"},
				"unit":      map[string]any{"type": "string", "description": "Unit type (optional)"},
				"storclass": map[string]any{"type": "string", "description": "DFSMS storage class (optional)"},
				"mgmtclass": map[string]any{"type": "string", "description": "DFSMS management class (optional)"},
				"dataclass": map[string]any{"type": "string", "description": "DFSMS data class (optional)"},
				"dsntype":   map[string]any{"type": "string", "description": "Dataset name type, e.g. 'EXTREQ' for PDSE (optional)"},
			},
			"required": []string{"system", "dataset", "dsorg", "alcunit", "primary", "secondary", "recfm", "lrecl"},
		}),
	}
}

func (t *createDatasetTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args createDatasetArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	req := zosmf.CreateDatasetRequest{
		DSOrg:     args.DSOrg,
		AlcUnit:   args.AlcUnit,
		Primary:   args.Primary,
		Secondary: args.Secondary,
		RecFm:     args.RecFm,
		LRecL:     args.LRecL,
		BlkSize:   args.BlkSize,
		DirBlk:    args.DirBlk,
		VolSer:    args.VolSer,
		Unit:      args.Unit,
		StorClass: args.StorClass,
		MgmtClass: args.MgmtClass,
		DataClass: args.DataClass,
		DSNType:   args.DSNType,
	}

	if err := client.CreateDataset(args.Dataset, req); err != nil {
		return mcp.ErrorResult("failed to create dataset: " + err.Error()), nil
	}

	return mcp.TextResult(fmt.Sprintf("Dataset %s created successfully", args.Dataset)), nil
}

// --- Delete Dataset ---

type deleteDatasetTool struct {
	clients *zosmf.ClientRegistry
}

type deleteDatasetArgs struct {
	System  string `json:"system"`
	Dataset string `json:"dataset"`
}

func NewDeleteDatasetTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &deleteDatasetTool{clients: clients}
}

func (t *deleteDatasetTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_delete_dataset",
		Description: "Delete a z/OS dataset or PDS member. WARNING: Permanent deletion. For a PDS member use 'DATASET.NAME(MEMBER)' format.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":  map[string]any{"type": "string", "description": "Name of the z/OS system"},
				"dataset": map[string]any{"type": "string", "description": "Dataset name or PDS member (e.g. 'USER.DATA' or 'USER.PDS(MEMBER)')"},
			},
			"required": []string{"system", "dataset"},
		}),
	}
}

func (t *deleteDatasetTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args deleteDatasetArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	if err := client.DeleteDataset(args.Dataset); err != nil {
		return mcp.ErrorResult("failed to delete dataset: " + err.Error()), nil
	}

	return mcp.TextResult(fmt.Sprintf("Dataset %s deleted successfully", args.Dataset)), nil
}

// --- Acquire Dataset ENQ ---

type acquireDatasetEnqTool struct {
	clients *zosmf.ClientRegistry
}

type acquireDatasetEnqArgs struct {
	System   string `json:"system"`
	Dataset  string `json:"dataset"`
	EnqType  string `json:"enq_type"`
	Encoding string `json:"encoding,omitempty"`
}

func NewAcquireDatasetEnqTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &acquireDatasetEnqTool{clients: clients}
}

func (t *acquireDatasetEnqTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_acquire_dataset_enq",
		Description: "Acquire an ENQ lock on a z/OS dataset or PDS member without reading its content. " +
			"Use enq_type 'exclusive' (SHRW) to prevent other readers and writers, or 'shared' (SHRE) to allow concurrent readers. " +
			"Returns a session_ref that must be passed to zosmf_release_dataset_enq when done, " +
			"and to zosmf_read_dataset / zosmf_write_dataset to associate operations with this ENQ session. " +
			"Returns an ENQ_NOT_AVAILABLE error if the dataset is already locked.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":  map[string]any{"type": "string", "description": "Name of the z/OS system"},
				"dataset": map[string]any{"type": "string", "description": "Dataset name, e.g. 'HLQ.PDS(MEMBER)' or 'HLQ.SEQ'"},
				"enq_type": map[string]any{
					"type":        "string",
					"description": "Lock type: 'exclusive' (SHRW — no other readers or writers) or 'shared' (SHRE — concurrent readers allowed)",
					"enum":        []string{"exclusive", "shared"},
				},
				"encoding": map[string]any{
					"type":        "string",
					"description": "Optional EBCDIC code page for z/OSMF conversion, e.g. 'IBM-1141'. Omit to use z/OSMF default (IBM-1047).",
				},
			},
			"required": []string{"system", "dataset", "enq_type"},
		}),
	}
}

func (t *acquireDatasetEnqTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args acquireDatasetEnqArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	content, sessionRef, err := client.AcquireDatasetEnq(args.Dataset, args.EnqType, args.Encoding)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	data, _ := json.Marshal(map[string]string{"session_ref": sessionRef, "content": content})
	return mcp.TextResult(string(data)), nil
}

// --- Release Dataset ENQ ---

type releaseDatasetEnqTool struct {
	clients *zosmf.ClientRegistry
}

type releaseDatasetEnqArgs struct {
	System     string `json:"system"`
	Dataset    string `json:"dataset"`
	SessionRef string `json:"session_ref"`
}

func NewReleaseDatasetEnqTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &releaseDatasetEnqTool{clients: clients}
}

func (t *releaseDatasetEnqTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_release_dataset_enq",
		Description: "Release a previously acquired ENQ lock on a z/OS dataset. Pass the session_ref returned by zosmf_acquire_dataset_enq.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system":      map[string]any{"type": "string", "description": "Name of the z/OS system"},
				"dataset":     map[string]any{"type": "string", "description": "Same dataset name used when acquiring the ENQ"},
				"session_ref": map[string]any{"type": "string", "description": "Session reference returned by zosmf_acquire_dataset_enq"},
			},
			"required": []string{"system", "dataset", "session_ref"},
		}),
	}
}

func (t *releaseDatasetEnqTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args releaseDatasetEnqArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	if err := client.ReleaseDatasetEnq(args.Dataset, args.SessionRef); err != nil {
		// Non-fatal: log but don't error — ENQ will time out on z/OS
		w, _ := json.Marshal(map[string]string{"warning": "release ENQ failed (non-fatal): " + err.Error()})
		return mcp.TextResult(string(w)), nil
	}

	return mcp.TextResult(`{}`), nil
}
