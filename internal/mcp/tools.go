package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ToolHandler is the interface that tool implementations must satisfy.
type ToolHandler interface {
	Definition() Tool
	Execute(ctx context.Context, arguments json.RawMessage) (ToolCallResult, error)
}

// ToolRegistry manages registered MCP tools.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]ToolHandler
	order []string // preserves registration order
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]ToolHandler),
	}
}

func (r *ToolRegistry) Register(handler ToolHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := handler.Definition().Name
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool already registered: %s", name)
	}
	r.tools[name] = handler
	r.order = append(r.order, name)
	return nil
}

func (r *ToolRegistry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		tools = append(tools, r.tools[name].Definition())
	}
	return tools
}

func (r *ToolRegistry) Get(name string) (ToolHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	h, ok := r.tools[name]
	return h, ok
}

func (r *ToolRegistry) HandleList(params json.RawMessage) (*ToolsListResult, error) {
	// For now, no pagination — return all tools at once
	return &ToolsListResult{
		Tools: r.List(),
	}, nil
}

func (r *ToolRegistry) HandleCall(ctx context.Context, params json.RawMessage) (*ToolCallResult, error) {
	var p ToolCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid tool call params: %w", err)
	}

	handler, ok := r.Get(p.Name)
	if !ok {
		result := ErrorResult(fmt.Sprintf("unknown tool: %s", p.Name))
		return &result, nil
	}

	result, err := handler.Execute(ctx, p.Arguments)
	if err != nil {
		result := ErrorResult(fmt.Sprintf("tool execution error: %v", err))
		return &result, nil
	}

	return &result, nil
}
