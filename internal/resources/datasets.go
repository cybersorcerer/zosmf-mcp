package resources

import (
	"context"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

type datasetHandler struct {
	clients *zosmf.ClientRegistry
}

func NewDatasetHandler(clients *zosmf.ClientRegistry) ResourceHandler {
	return &datasetHandler{clients: clients}
}

func (h *datasetHandler) Definitions() []mcp.Resource { return nil }

func (h *datasetHandler) Templates() []mcp.ResourceTemplate {
	return []mcp.ResourceTemplate{{
		URITemplate: "zosmf://{system}/dataset/{dsn}",
		Name:        "Dataset or PDS member",
		Description: "A sequential dataset or a PDS member, for example zosmf://S0W1/dataset/SYS1.PARMLIB(IEASYS00).",
		MIMEType:    "text/plain",
	}}
}

func (h *datasetHandler) Matches(uri string) bool {
	ref, err := ParseURI(uri)
	return err == nil && ref.Kind == KindDataset
}

func (h *datasetHandler) Read(_ context.Context, uri string) (mcp.ResourceContents, error) {
	ref, err := ParseURI(uri)
	if err != nil {
		return mcp.ResourceContents{}, err
	}

	client, err := h.clients.Get(ref.System)
	if err != nil {
		return mcp.ResourceContents{}, err
	}

	body, err := client.ReadDataset(ref.Dataset, "")
	if err != nil {
		return mcp.ResourceContents{}, describeZOSMFError(err, "dataset "+ref.Dataset)
	}

	return buildContents(uri, "text/plain", body)
}
