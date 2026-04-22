package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
)

type serverState int

const (
	stateUninitialized serverState = iota
	stateInitializing
	stateReady
)

const (
	ServerName    = "zosmf-mcp"
	ServerVersion = "0.3.0"
)

// Transport defines how the server sends and receives messages.
type Transport interface {
	// Run starts the transport, reading messages and passing them to the handler.
	// It blocks until the transport is closed or the context is cancelled.
	Run(ctx context.Context, handler MessageHandler) error
}

// MessageHandler processes an incoming JSON-RPC message and returns a response.
// For notifications, it returns nil.
type MessageHandler func(ctx context.Context, msg *JSONRPCMessage) *JSONRPCMessage

// Server is the MCP server that manages lifecycle, tools, and message dispatch.
type Server struct {
	mu       sync.RWMutex
	state    serverState
	registry *ToolRegistry
	logger   *log.Logger
	httpMode bool // when true, initialize resets state per-call (HTTP transport is session-aware)
}

func NewServer(registry *ToolRegistry, logger *log.Logger) *Server {
	return &Server{
		state:    stateUninitialized,
		registry: registry,
		logger:   logger,
	}
}

func NewHTTPServer(registry *ToolRegistry, logger *log.Logger) *Server {
	return &Server{
		state:    stateReady, // HTTP transport manages session state itself
		registry: registry,
		logger:   logger,
		httpMode: true,
	}
}

// HandleMessage is the main message dispatcher.
func (s *Server) HandleMessage(ctx context.Context, msg *JSONRPCMessage) *JSONRPCMessage {
	// Handle batch messages
	if msg == nil {
		return NewErrorResponse(nil, ErrCodeInvalidRequest, "empty message")
	}

	// Notifications don't get responses
	if msg.IsNotification() {
		s.handleNotification(msg)
		return nil
	}

	// Responses are ignored (we don't send requests to clients yet)
	if msg.IsResponse() {
		return nil
	}

	// Must be a request
	if !msg.IsRequest() {
		return NewErrorResponse(msg.ID, ErrCodeInvalidRequest, "invalid message")
	}

	return s.handleRequest(ctx, msg)
}

func (s *Server) handleRequest(ctx context.Context, msg *JSONRPCMessage) *JSONRPCMessage {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	// ping is always allowed
	if msg.Method == "ping" {
		resp, err := NewResponse(msg.ID, PingResult{})
		if err != nil {
			return NewErrorResponse(msg.ID, ErrCodeInternal, err.Error())
		}
		return resp
	}

	// In HTTP mode the transport manages session state — initialize is always valid
	if s.httpMode {
		switch msg.Method {
		case "initialize":
			return s.handleInitialize(msg)
		case "tools/list":
			return s.handleToolsList(msg)
		case "tools/call":
			return s.handleToolsCall(ctx, msg)
		default:
			return NewErrorResponse(msg.ID, ErrCodeMethodNotFound,
				fmt.Sprintf("unknown method: %s", msg.Method))
		}
	}

	// Before initialization, only initialize is allowed
	if state == stateUninitialized {
		if msg.Method != "initialize" {
			return NewErrorResponse(msg.ID, ErrCodeInvalidRequest,
				"server not initialized — send 'initialize' first")
		}
		return s.handleInitialize(msg)
	}

	// During initialization (waiting for initialized notification), reject other requests
	if state == stateInitializing {
		return NewErrorResponse(msg.ID, ErrCodeInvalidRequest,
			"server waiting for 'notifications/initialized'")
	}

	// Server is ready — dispatch to method handlers
	switch msg.Method {
	case "initialize":
		return NewErrorResponse(msg.ID, ErrCodeInvalidRequest, "server already initialized")
	case "tools/list":
		return s.handleToolsList(msg)
	case "tools/call":
		return s.handleToolsCall(ctx, msg)
	default:
		return NewErrorResponse(msg.ID, ErrCodeMethodNotFound,
			fmt.Sprintf("unknown method: %s", msg.Method))
	}
}

func (s *Server) handleNotification(msg *JSONRPCMessage) {
	switch msg.Method {
	case "notifications/initialized":
		s.mu.Lock()
		if s.state == stateInitializing {
			s.state = stateReady
			s.logger.Println("server initialized and ready")
		}
		s.mu.Unlock()
	case "notifications/cancelled":
		// Client cancelled a request — we could handle this in the future
		s.logger.Printf("received cancellation notification")
	default:
		s.logger.Printf("unknown notification: %s", msg.Method)
	}
}

func (s *Server) handleInitialize(msg *JSONRPCMessage) *JSONRPCMessage {
	var params InitializeParams
	if msg.Params != nil {
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return NewErrorResponse(msg.ID, ErrCodeInvalidParams, "invalid initialize params")
		}
	}

	s.logger.Printf("initialize request from %s %s (protocol %s)",
		params.ClientInfo.Name, params.ClientInfo.Version, params.ProtocolVersion)

	s.mu.Lock()
	s.state = stateInitializing
	s.mu.Unlock()

	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{
				ListChanged: true,
			},
		},
		ServerInfo: Implementation{
			Name:    ServerName,
			Version: ServerVersion,
		},
		Instructions: "z/OSMF MCP Server — provides access to z/OS systems via z/OSMF REST API. " +
			"Available tools allow you to manage jobs and query SMP/E CSI data.",
	}

	resp, err := NewResponse(msg.ID, result)
	if err != nil {
		return NewErrorResponse(msg.ID, ErrCodeInternal, err.Error())
	}
	return resp
}

func (s *Server) handleToolsList(msg *JSONRPCMessage) *JSONRPCMessage {
	result, err := s.registry.HandleList(msg.Params)
	if err != nil {
		return NewErrorResponse(msg.ID, ErrCodeInternal, err.Error())
	}

	resp, err := NewResponse(msg.ID, result)
	if err != nil {
		return NewErrorResponse(msg.ID, ErrCodeInternal, err.Error())
	}
	return resp
}

func (s *Server) handleToolsCall(ctx context.Context, msg *JSONRPCMessage) *JSONRPCMessage {
	if msg.Params == nil {
		return NewErrorResponse(msg.ID, ErrCodeInvalidParams, "missing params")
	}

	result, err := s.registry.HandleCall(ctx, msg.Params)
	if err != nil {
		return NewErrorResponse(msg.ID, ErrCodeInternal, err.Error())
	}

	resp, err := NewResponse(msg.ID, result)
	if err != nil {
		return NewErrorResponse(msg.ID, ErrCodeInternal, err.Error())
	}
	return resp
}
