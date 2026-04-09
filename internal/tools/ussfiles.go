package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

// --- List USS Files ---

type listUSSFilesTool struct {
	clients *zosmf.ClientRegistry
}

type listUSSFilesArgs struct {
	System string `json:"system"`
	Path   string `json:"path"`
}

func NewListUSSFilesTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &listUSSFilesTool{clients: clients}
}

func (t *listUSSFilesTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_list_uss_files",
		Description: "List files and directories in a z/OS Unix System Services (USS) directory.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{
					"type":        "string",
					"description": "Name of the z/OS system",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute USS path to list (e.g. '/u/ibmuser', '/usr/lpp')",
				},
			},
			"required": []string{"system", "path"},
		}),
	}
}

func (t *listUSSFilesTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args listUSSFilesArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	files, err := client.ListUSSFiles(args.Path)
	if err != nil {
		return mcp.ErrorResult("failed to list USS files: " + err.Error()), nil
	}

	data, _ := json.MarshalIndent(files, "", "  ")
	return mcp.TextResult(string(data)), nil
}

// --- Read USS File ---

type readUSSFileTool struct {
	clients *zosmf.ClientRegistry
}

type readUSSFileArgs struct {
	System string `json:"system"`
	Path   string `json:"path"`
}

func NewReadUSSFileTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &readUSSFileTool{clients: clients}
}

func (t *readUSSFileTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_read_uss_file",
		Description: "Read the content of a z/OS USS file as text.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{
					"type":        "string",
					"description": "Name of the z/OS system",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute USS file path (e.g. '/u/ibmuser/.profile')",
				},
			},
			"required": []string{"system", "path"},
		}),
	}
}

func (t *readUSSFileTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args readUSSFileArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	content, err := client.ReadUSSFile(args.Path)
	if err != nil {
		return mcp.ErrorResult("failed to read USS file: " + err.Error()), nil
	}

	return mcp.TextResult(content), nil
}

// --- Write USS File ---

type writeUSSFileTool struct {
	clients *zosmf.ClientRegistry
}

type writeUSSFileArgs struct {
	System  string `json:"system"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

func NewWriteUSSFileTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &writeUSSFileTool{clients: clients}
}

func (t *writeUSSFileTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_write_uss_file",
		Description: "Write text content to a z/OS USS file. WARNING: Overwrites the existing content.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{
					"type":        "string",
					"description": "Name of the z/OS system",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute USS file path (e.g. '/u/ibmuser/myscript.sh')",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Text content to write to the file",
				},
			},
			"required": []string{"system", "path", "content"},
		}),
	}
}

func (t *writeUSSFileTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args writeUSSFileArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	if err := client.WriteUSSFile(args.Path, args.Content); err != nil {
		return mcp.ErrorResult("failed to write USS file: " + err.Error()), nil
	}

	return mcp.TextResult(fmt.Sprintf("Successfully wrote to %s", args.Path)), nil
}

// --- Create USS File/Directory ---

type createUSSFileTool struct {
	clients *zosmf.ClientRegistry
}

type createUSSFileArgs struct {
	System   string `json:"system"`
	Path     string `json:"path"`
	FileType string `json:"file_type"`
	Mode     string `json:"mode,omitempty"`
}

func NewCreateUSSFileTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &createUSSFileTool{clients: clients}
}

func (t *createUSSFileTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_create_uss_file",
		Description: "Create a new z/OS USS file or directory.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{
					"type":        "string",
					"description": "Name of the z/OS system",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute USS path for the new file or directory",
				},
				"file_type": map[string]any{
					"type":        "string",
					"description": "Type to create: 'file' or 'dir'",
					"enum":        []string{"file", "dir"},
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "Permissions string (e.g. 'rwxr-xr-x'). Defaults to 'rw-r--r--' for files, 'rwxr-xr-x' for directories.",
				},
			},
			"required": []string{"system", "path", "file_type"},
		}),
	}
}

func (t *createUSSFileTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args createUSSFileArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	if err := client.CreateUSSFile(args.Path, args.FileType, args.Mode); err != nil {
		return mcp.ErrorResult("failed to create USS file: " + err.Error()), nil
	}

	return mcp.TextResult(fmt.Sprintf("Successfully created %s %s", args.FileType, args.Path)), nil
}

// --- Delete USS File ---

type deleteUSSFileTool struct {
	clients *zosmf.ClientRegistry
}

type deleteUSSFileArgs struct {
	System    string `json:"system"`
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}

func NewDeleteUSSFileTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &deleteUSSFileTool{clients: clients}
}

func (t *deleteUSSFileTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_delete_uss_file",
		Description: "Delete a z/OS USS file or directory. WARNING: Permanent deletion.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{
					"type":        "string",
					"description": "Name of the z/OS system",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute USS path to delete",
				},
				"recursive": map[string]any{
					"type":        "boolean",
					"description": "If true, delete non-empty directories recursively. Default: false.",
				},
			},
			"required": []string{"system", "path"},
		}),
	}
}

func (t *deleteUSSFileTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args deleteUSSFileArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	if err := client.DeleteUSSFile(args.Path, args.Recursive); err != nil {
		return mcp.ErrorResult("failed to delete USS file: " + err.Error()), nil
	}

	return mcp.TextResult(fmt.Sprintf("Successfully deleted %s", args.Path)), nil
}
