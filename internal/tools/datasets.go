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
}

func NewListDatasetsTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &listDatasetsTool{clients: clients}
}

func (t *listDatasetsTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_list_datasets",
		Description: "List z/OS datasets matching a pattern. Use standard z/OS dataset level patterns " +
			"(e.g. 'SYS1.PARM*', 'USER.**.JCL'). Returns dataset names with attributes like DSORG, RECFM, LRECL.",
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

	datasets, err := client.ListDatasets(args.DSLevel)
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
		Description: "List members of a PDS or PDSE dataset on z/OS.",
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
	System  string `json:"system"`
	Dataset string `json:"dataset"`
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

	content, err := client.ReadDataset(args.Dataset)
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
	System  string `json:"system"`
	Dataset string `json:"dataset"`
	Content string `json:"content"`
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

	if err := client.WriteDataset(args.Dataset, args.Content); err != nil {
		return mcp.ErrorResult("failed to write dataset: " + err.Error()), nil
	}

	return mcp.TextResult(fmt.Sprintf("Successfully wrote to %s", args.Dataset)), nil
}
