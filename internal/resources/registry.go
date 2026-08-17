package resources

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

// ErrNotFound is returned when no handler claims a URI. The server maps it
// to JSON-RPC -32002.
var ErrNotFound = errors.New("resource not found")

// ResourceHandler serves one family of resources. Handlers must claim
// disjoint URI spaces: the registry resolves a read by asking each handler
// in registration order and taking the first that matches, so two handlers
// claiming the same URI would leave the second one's resources unreachable.
// The registry cannot detect that — URI templates are not literal strings —
// so it is the handler authors' contract to keep.
type ResourceHandler interface {
	// Definitions returns concrete, listable resources. Most handlers
	// return nil: a system's datasets cannot be enumerated.
	Definitions() []mcp.Resource

	// Templates returns the URI templates this handler serves.
	Templates() []mcp.ResourceTemplate

	// Matches reports whether this handler serves the given URI.
	Matches(uri string) bool

	// Read fetches the content behind the URI.
	Read(ctx context.Context, uri string) (mcp.ResourceContents, error)
}

// ResourceRegistry routes resource URIs to handlers.
type ResourceRegistry struct {
	mu       sync.RWMutex
	handlers []ResourceHandler
	seen     map[string]bool // guards against two handlers claiming one URI
}

func NewResourceRegistry() *ResourceRegistry {
	return &ResourceRegistry{seen: make(map[string]bool)}
}

func (r *ResourceRegistry) Register(h ResourceHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	defs := h.Definitions()
	for _, def := range defs {
		if r.seen[def.URI] {
			return fmt.Errorf("resource already registered: %s", def.URI)
		}
	}
	for _, def := range defs {
		r.seen[def.URI] = true
	}
	r.handlers = append(r.handlers, h)
	return nil
}

func (r *ResourceRegistry) List() []mcp.Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []mcp.Resource
	for _, h := range r.handlers {
		out = append(out, h.Definitions()...)
	}
	return out
}

func (r *ResourceRegistry) Templates() []mcp.ResourceTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []mcp.ResourceTemplate
	for _, h := range r.handlers {
		out = append(out, h.Templates()...)
	}
	return out
}

// Read resolves a URI to its content by asking each handler in registration
// order and taking the first match.
func (r *ResourceRegistry) Read(ctx context.Context, uri string) (mcp.ResourceContents, error) {
	r.mu.RLock()
	handlers := make([]ResourceHandler, len(r.handlers))
	copy(handlers, r.handlers)
	r.mu.RUnlock()

	for _, h := range handlers {
		if h.Matches(uri) {
			return h.Read(ctx, uri)
		}
	}
	return mcp.ResourceContents{}, fmt.Errorf("%w: %s", ErrNotFound, uri)
}

// RegisterAll registers every resource handler with the given registry.
func RegisterAll(registry *ResourceRegistry, clients *zosmf.ClientRegistry, cfg *config.Config) error {
	handlers := []ResourceHandler{
		NewSystemsHandler(clients, cfg),
		NewDatasetHandler(clients),
		NewUSSHandler(clients),
		NewJobSpoolHandler(clients),
	}

	for _, h := range handlers {
		if err := registry.Register(h); err != nil {
			return err
		}
	}
	return nil
}
