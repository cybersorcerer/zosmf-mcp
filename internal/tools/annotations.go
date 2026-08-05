package tools

import (
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
)

// readOnlyTool describes a tool that only queries state. Clients may run
// these without asking the user.
func readOnlyTool(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

// writeTool describes a tool that changes state on the z/OS system.
// destructive: the change can lose data or interrupt running work.
// idempotent: calling it twice with the same arguments has the same effect
// as calling it once.
// openWorld: the effect is not bounded by the tool's own domain — true for
// tools that pass an arbitrary command through to the system.
func writeTool(title string, destructive, idempotent, openWorld bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		ReadOnlyHint:    false,
		DestructiveHint: destructive,
		IdempotentHint:  idempotent,
		OpenWorldHint:   openWorld,
	}
}

// systemProperty builds the 'system' input schema property for tools that
// talk to z/OSMF. The enum is omitted when no systems are configured —
// an empty enum would match nothing and make every call invalid.
func systemProperty(names []string) map[string]any {
	prop := map[string]any{
		"type": "string",
		"description": "Name of the z/OS system to act on, as defined in the 'systems:' block " +
			"of the server configuration. Case-insensitive.",
	}
	if len(names) > 0 {
		prop["enum"] = names
	}
	return prop
}

// sshSystemProperty builds the 'ssh_system' property for the ZOAU tools.
// These resolve against a different configuration block than systemProperty,
// so the description spells the difference out.
func sshSystemProperty(names []string) map[string]any {
	prop := map[string]any{
		"type": "string",
		"description": "Name of the z/OS system to reach over SSH, as defined in the 'ssh_systems:' " +
			"block of the server configuration. This is a separate namespace from the 'system' " +
			"parameter of the zosmf_* tools, which resolves against 'systems:'. Case-insensitive.",
	}
	if len(names) > 0 {
		prop["enum"] = names
	}
	return prop
}
