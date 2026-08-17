package resources

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
)

// ErrNotFound is returned when no handler claims a URI. The server maps it
// to JSON-RPC -32002.
var ErrNotFound = errors.New("resource not found")

// ResourceHandler serves one family of resources.
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

	for _, def := range h.Definitions() {
		if r.seen[def.URI] {
			return fmt.Errorf("resource already registered: %s", def.URI)
		}
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
