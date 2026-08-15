package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

// --- List Filesystems ---

type listFilesystemsTool struct {
	clients *zosmf.ClientRegistry
}

type listFilesystemsArgs struct {
	System string `json:"system"`
	FSName string `json:"fsname,omitempty"`
	Path   string `json:"path,omitempty"`
}

func NewListFilesystemsTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &listFilesystemsTool{clients: clients}
}

func (t *listFilesystemsTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_list_filesystems",
		Description: "List mounted z/OS filesystems (zFS, HFS, etc.). Returns for each filesystem: dataset name, " +
			"mount point, filesystem type, status, mount mode flags, device number, owning system name, " +
			"and the read/write/directory I/O block counts. " +
			"Filter by filesystem name (wildcards allowed) or by mount path.",
		Annotations: readOnlyTool("List Filesystems"),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": systemProperty(t.clients.SystemNames()),
				"fsname": map[string]any{
					"type":        "string",
					"description": "Filter by filesystem name (e.g. 'OMVS.ZFS*'). Supports wildcards.",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Filter by mount path (e.g. '/usr')",
				},
			},
			"required": []string{"system"},
		}),
	}
}

func (t *listFilesystemsTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args listFilesystemsArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	filesystems, err := client.ListFilesystems(args.FSName, args.Path)
	if err != nil {
		return mcp.ErrorResult("failed to list filesystems: " + err.Error()), nil
	}

	data, _ := json.MarshalIndent(filesystems, "", "  ")
	return mcp.TextResult(string(data)), nil
}

// --- Mount Filesystem ---

type mountFilesystemTool struct {
	clients *zosmf.ClientRegistry
}

type mountFilesystemArgs struct {
	System     string `json:"system"`
	FSName     string `json:"fsname"`
	MountPoint string `json:"mount_point"`
	FSType     string `json:"fs_type"`
	Mode       string `json:"mode,omitempty"`
}

func NewMountFilesystemTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &mountFilesystemTool{clients: clients}
}

func (t *mountFilesystemTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_mount_filesystem",
		Description: "Mount a z/OS filesystem (zFS, HFS, etc.) at a USS mount point. " +
			"The mount point directory must already exist. Fails if the filesystem is already mounted. " +
			"Defaults to read-write ('rdwr') unless mode says otherwise.",
		Annotations: writeTool("Mount Filesystem", false, false, false),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": systemProperty(t.clients.SystemNames()),
				"fsname": map[string]any{
					"type":        "string",
					"description": "Filesystem dataset name to mount (e.g. 'OMVS.ROOT.ZFS')",
				},
				"mount_point": map[string]any{
					"type":        "string",
					"description": "USS mount point directory (e.g. '/mnt/myfs')",
				},
				"fs_type": map[string]any{
					"type":        "string",
					"description": "Filesystem type (e.g. 'zfs', 'hfs')",
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "Mount mode options (e.g. 'rdwr', 'rdonly', 'rdwr nosetuid'). Defaults to 'rdwr'.",
				},
			},
			"required": []string{"system", "fsname", "mount_point", "fs_type"},
		}),
	}
}

func (t *mountFilesystemTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args mountFilesystemArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	mode := args.Mode
	if mode == "" {
		mode = "rdwr"
	}

	if err := client.MountFilesystem(args.FSName, args.MountPoint, args.FSType, mode); err != nil {
		return mcp.ErrorResult("failed to mount filesystem: " + err.Error()), nil
	}

	return mcp.TextResult(fmt.Sprintf("Filesystem %s mounted at %s", args.FSName, args.MountPoint)), nil
}

// --- Unmount Filesystem ---

type unmountFilesystemTool struct {
	clients *zosmf.ClientRegistry
}

type unmountFilesystemArgs struct {
	System string `json:"system"`
	FSName string `json:"fsname"`
}

func NewUnmountFilesystemTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &unmountFilesystemTool{clients: clients}
}

func (t *unmountFilesystemTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_unmount_filesystem",
		Description: "Unmount a mounted z/OS filesystem. WARNING: This makes the filesystem's contents " +
			"inaccessible to every job and user on the system, and fails if files under the mount point " +
			"are still in use.",
		Annotations: writeTool("Unmount Filesystem", true, true, false),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": systemProperty(t.clients.SystemNames()),
				"fsname": map[string]any{
					"type":        "string",
					"description": "Filesystem dataset name to unmount (e.g. 'OMVS.ROOT.ZFS')",
				},
			},
			"required": []string{"system", "fsname"},
		}),
	}
}

func (t *unmountFilesystemTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args unmountFilesystemArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	if err := client.UnmountFilesystem(args.FSName); err != nil {
		return mcp.ErrorResult("failed to unmount filesystem: " + err.Error()), nil
	}

	return mcp.TextResult(fmt.Sprintf("Filesystem %s unmounted successfully", args.FSName)), nil
}

// --- Create zFS ---

type createZFSTool struct {
	clients *zosmf.ClientRegistry
}

type createZFSArgs struct {
	System          string   `json:"system"`
	ZFSName         string   `json:"zfsname"`
	CylsPri         int      `json:"cyls_pri"`
	CylsSec         int      `json:"cyls_sec,omitempty"`
	Owner           string   `json:"owner,omitempty"`
	Group           string   `json:"group,omitempty"`
	Perms           int      `json:"perms,omitempty"`
	StorageClass    string   `json:"storage_class,omitempty"`
	ManagementClass string   `json:"management_class,omitempty"`
	DataClass       string   `json:"data_class,omitempty"`
	Volumes         []string `json:"volumes,omitempty"`
}

func NewCreateZFSTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &createZFSTool{clients: clients}
}

func (t *createZFSTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_create_zfs",
		Description: "Create a new zFS (z/OS File System) dataset. Allocates the backing dataset only — " +
			"it is not mounted afterwards; use zosmf_mount_filesystem for that. " +
			"Space is given in cylinders. Fails if the dataset already exists.",
		Annotations: writeTool("Create zFS", false, false, false),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": systemProperty(t.clients.SystemNames()),
				"zfsname": map[string]any{
					"type":        "string",
					"description": "Dataset name for the new zFS (e.g. 'USER.MY.ZFS')",
				},
				"cyls_pri": map[string]any{
					"type":        "integer",
					"description": "Primary space allocation in cylinders",
				},
				"cyls_sec": map[string]any{
					"type":        "integer",
					"description": "Secondary space allocation in cylinders (optional)",
				},
				"owner": map[string]any{
					"type":        "string",
					"description": "USS owner user ID (optional)",
				},
				"group": map[string]any{
					"type":        "string",
					"description": "USS owner group ID (optional)",
				},
				"perms": map[string]any{
					"type":        "integer",
					"description": "USS permissions in octal (e.g. 755, optional)",
				},
				"storage_class": map[string]any{
					"type":        "string",
					"description": "DFSMS storage class (optional)",
				},
				"management_class": map[string]any{
					"type":        "string",
					"description": "DFSMS management class (optional)",
				},
				"data_class": map[string]any{
					"type":        "string",
					"description": "DFSMS data class (optional)",
				},
				"volumes": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "List of volume serial numbers (optional)",
				},
			},
			"required": []string{"system", "zfsname", "cyls_pri"},
		}),
	}
}

func (t *createZFSTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args createZFSArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	req := zosmf.CreateZFSRequest{
		CylsPri:         args.CylsPri,
		CylsSec:         args.CylsSec,
		Owner:           args.Owner,
		Group:           args.Group,
		Perms:           args.Perms,
		StorageClass:    args.StorageClass,
		ManagementClass: args.ManagementClass,
		DataClass:       args.DataClass,
		Volumes:         args.Volumes,
	}

	if err := client.CreateZFS(args.ZFSName, req); err != nil {
		return mcp.ErrorResult("failed to create zFS: " + err.Error()), nil
	}

	return mcp.TextResult(fmt.Sprintf("zFS %s created successfully", args.ZFSName)), nil
}

// --- Delete zFS ---

type deleteZFSTool struct {
	clients *zosmf.ClientRegistry
}

type deleteZFSArgs struct {
	System  string `json:"system"`
	ZFSName string `json:"zfsname"`
}

func NewDeleteZFSTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &deleteZFSTool{clients: clients}
}

func (t *deleteZFSTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "zosmf_delete_zfs",
		Description: "Delete a zFS dataset. WARNING: This permanently deletes the filesystem and all its contents.",
		Annotations: writeTool("Delete zFS", true, true, false),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": systemProperty(t.clients.SystemNames()),
				"zfsname": map[string]any{
					"type":        "string",
					"description": "Dataset name of the zFS to delete (e.g. 'USER.MY.ZFS')",
				},
			},
			"required": []string{"system", "zfsname"},
		}),
	}
}

func (t *deleteZFSTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args deleteZFSArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	if err := client.DeleteZFS(args.ZFSName); err != nil {
		return mcp.ErrorResult("failed to delete zFS: " + err.Error()), nil
	}

	return mcp.TextResult(fmt.Sprintf("zFS %s deleted successfully", args.ZFSName)), nil
}
