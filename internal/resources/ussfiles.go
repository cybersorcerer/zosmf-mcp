package resources

import (
	"context"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

type ussHandler struct {
	clients *zosmf.ClientRegistry
}

func NewUSSHandler(clients *zosmf.ClientRegistry) ResourceHandler {
	return &ussHandler{clients: clients}
}

func (h *ussHandler) Definitions() []mcp.Resource { return nil }

func (h *ussHandler) Templates() []mcp.ResourceTemplate {
	return []mcp.ResourceTemplate{{
		URITemplate: "zosmf://{system}/uss/{path}",
		Name:        "USS file",
		Description: "A z/OS UNIX file, for example zosmf://S0W1/uss/etc/profile. The path is case-sensitive.",
		MIMEType:    "text/plain",
	}}
}

func (h *ussHandler) Matches(uri string) bool {
	ref, err := ParseURI(uri)
	return err == nil && ref.Kind == KindUSS
}

func (h *ussHandler) Read(_ context.Context, uri string) (mcp.ResourceContents, error) {
	ref, err := ParseURI(uri)
	if err != nil {
		return mcp.ResourceContents{}, err
	}

	client, err := h.clients.Get(ref.System)
	if err != nil {
		return mcp.ResourceContents{}, err
	}

	body, err := client.ReadUSSFile(ref.Path, "")
	if err != nil {
		return mcp.ResourceContents{}, describeZOSMFError(err, "USS path "+ref.Path)
	}

	return buildContents(uri, "text/plain", body)
}
