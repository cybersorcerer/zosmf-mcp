package tools

import (
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/config"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

// RegisterAll registers all available MCP tools with the given registry.
// ZOAU tools are only registered if sshReg is non-nil (i.e. ssh_systems is configured).
func RegisterAll(registry *mcp.ToolRegistry, clients *zosmf.ClientRegistry, cfg *config.Config, sshReg *zosmf.SSHClientRegistry) error {
	handlers := []mcp.ToolHandler{
		// Jobs tools
		NewListJobsTool(clients),
		NewGetJobTool(clients),
		NewGetJobOutputTool(clients),
		NewGetSpoolContentTool(clients),
		NewGetAllSpoolsTool(clients),
		NewSubmitJobTool(clients),
		NewCancelJobTool(clients),
		NewPurgeJobTool(clients),
		NewHoldJobTool(clients),
		NewReleaseJobTool(clients),
		// SMP/E tools
		NewQueryCSITool(clients, cfg),
		// Dataset tools
		NewListDatasetsTool(clients),
		NewListMembersTool(clients),
		NewReadDatasetTool(clients),
		NewWriteDatasetTool(clients),
		NewAcquireDatasetEnqTool(clients),
		NewReleaseDatasetEnqTool(clients),
		// Filesystem tools
		NewListFilesystemsTool(clients),
		NewMountFilesystemTool(clients),
		NewUnmountFilesystemTool(clients),
		NewCreateZFSTool(clients),
		NewDeleteZFSTool(clients),
		// Console tools
		NewIssueCommandTool(clients, nil),
		NewGetHardcopyLogTool(clients),
		// USS Files tools
		NewListUSSFilesTool(clients),
		NewReadUSSFileTool(clients),
		NewWriteUSSFileTool(clients),
		NewCreateUSSFileTool(clients),
		NewDeleteUSSFileTool(clients),
		// Extended Dataset tools
		NewCreateDatasetTool(clients),
		NewDeleteDatasetTool(clients),
		// TSO tool
		NewTSOCommandTool(clients, cfg),
		// Parmlib tools
		NewParmlibValidateTool(clients),
	}

	if sshReg != nil {
		handlers = append(handlers,
			NewZOAUPllTool(sshReg),
			NewZOAUZinfoTool(sshReg),
			NewZOAULlwhenceTool(sshReg),
			NewZOAUParmgrepTool(sshReg),
			NewZOAUParmwhenceTool(sshReg),
			NewZOAUPprocTool(sshReg),
			NewZOAUProcgrepTool(sshReg),
			NewZOAUProcwhenceTool(sshReg),
			NewZOAUVersionTool(sshReg),
			NewZOAUVtoclsTool(sshReg),
		)
	}

	for _, h := range handlers {
		if err := registry.Register(h); err != nil {
			return err
		}
	}

	return nil
}
