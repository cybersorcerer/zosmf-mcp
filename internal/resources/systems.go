package resources

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

// systemsHandler serves the server-level inventory. It is the only listable
// resource: everything else on a z/OS system needs a name to address it.
type systemsHandler struct {
	clients *zosmf.ClientRegistry
	cfg     *config.Config
}

// systemEntry is the shape written into the inventory. Deliberately narrow —
// no auth block, so credentials cannot leak into a resource body.
type systemEntry struct {
	Name        string   `json:"name"`
	Host        string   `json:"host"`
	CSIDatasets []string `json:"csi_datasets,omitempty"`
	TargetZones []string `json:"target_zones,omitempty"`
}

func NewSystemsHandler(clients *zosmf.ClientRegistry, cfg *config.Config) ResourceHandler {
	return &systemsHandler{clients: clients, cfg: cfg}
}

func (h *systemsHandler) Definitions() []mcp.Resource {
	return []mcp.Resource{{
		URI:         SystemsURI,
		Name:        "Configured z/OS systems",
		Description: "The z/OSMF endpoints this server talks to, with their SMP/E CSI datasets and target zones.",
		MIMEType:    "application/json",
	}}
}

func (h *systemsHandler) Templates() []mcp.ResourceTemplate { return nil }

func (h *systemsHandler) Matches(uri string) bool { return uri == SystemsURI }

func (h *systemsHandler) Read(_ context.Context, uri string) (mcp.ResourceContents, error) {
	if uri != SystemsURI {
		return mcp.ResourceContents{}, fmt.Errorf("%w: %s", ErrNotFound, uri)
	}

	entries := make([]systemEntry, 0, len(h.cfg.Systems))
	for _, s := range h.cfg.Systems {
		entries = append(entries, systemEntry{
			Name:        s.Name,
			Host:        s.Host,
			CSIDatasets: s.CSIDatasets,
			TargetZones: s.TargetZones,
		})
	}

	body, err := json.MarshalIndent(map[string]any{"systems": entries}, "", "  ")
	if err != nil {
		return mcp.ResourceContents{}, fmt.Errorf("building system inventory: %w", err)
	}

	return mcp.ResourceContents{
		URI:      SystemsURI,
		MIMEType: "application/json",
		Text:     string(body),
	}, nil
}
