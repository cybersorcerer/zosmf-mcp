package resources

import (
	"context"
	"fmt"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

type jobSpoolHandler struct {
	clients *zosmf.ClientRegistry
}

func NewJobSpoolHandler(clients *zosmf.ClientRegistry) ResourceHandler {
	return &jobSpoolHandler{clients: clients}
}

func (h *jobSpoolHandler) Definitions() []mcp.Resource { return nil }

func (h *jobSpoolHandler) Templates() []mcp.ResourceTemplate {
	return []mcp.ResourceTemplate{{
		URITemplate: "zosmf://{system}/job/{jobname}/{jobid}/spool/{id}",
		Name:        "Job spool file",
		Description: "One spool file of a job, addressed by the numeric id z/OSMF assigns. Several steps can share a DD name, so the id rather than the DD name identifies the file.",
		MIMEType:    "text/plain",
	}}
}

func (h *jobSpoolHandler) Matches(uri string) bool {
	ref, err := ParseURI(uri)
	return err == nil && ref.Kind == KindSpool
}

func (h *jobSpoolHandler) Read(_ context.Context, uri string) (mcp.ResourceContents, error) {
	ref, err := ParseURI(uri)
	if err != nil {
		return mcp.ResourceContents{}, err
	}

	client, err := h.clients.Get(ref.System)
	if err != nil {
		return mcp.ResourceContents{}, err
	}

	body, err := client.GetJobFileContent(ref.JobName, ref.JobID, ref.FileID)
	if err != nil {
		subject := fmt.Sprintf("spool file %d of job %s/%s", ref.FileID, ref.JobName, ref.JobID)
		return mcp.ResourceContents{}, describeZOSMFError(err, subject)
	}

	return buildContents(uri, "text/plain", body)
}
