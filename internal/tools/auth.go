package tools

import (
	"context"
	"encoding/json"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

// --- Set Password ---

type setPasswordTool struct {
	clients *zosmf.ClientRegistry
}

type setPasswordArgs struct {
	System   string `json:"system"`
	Password string `json:"password"`
}

func NewSetPasswordTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &setPasswordTool{clients: clients}
}

func (t *setPasswordTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_set_password",
		Description: "Set the z/OSMF password for a configured system at runtime. " +
			"The password is kept in memory only and is used for all subsequent " +
			"requests to that system. Use this when a tool call failed with a " +
			"NO_CREDENTIALS or AUTH_FAILED error. " +
			"The password is held in memory for this server process only and is not written to the " +
			"configuration file — it is lost on restart.",
		Annotations: writeTool("Set System Password", false, true, false),
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": systemProperty(t.clients.SystemNames()),
				"password": map[string]any{
					"type":        "string",
					"description": "Password for the system's configured username",
				},
			},
			"required": []string{"system", "password"},
		}),
	}
}

func (t *setPasswordTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args setPasswordArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}
	if args.System == "" || args.Password == "" {
		return mcp.ErrorResult("system and password are required"), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	client.SetPassword(args.Password)
	return mcp.TextResult("password updated for system " + args.System), nil
}
