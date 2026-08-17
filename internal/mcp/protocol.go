package mcp

import "encoding/json"

const ProtocolVersion = "2025-06-18"

// JSON-RPC 2.0 base types

type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Standard JSON-RPC error codes
const (
	ErrCodeParse          = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603
)

// ErrCodeResourceNotFound is the MCP-specific code for a resource URI that
// does not resolve.
const ErrCodeResourceNotFound = -32002

func NewRequest(id any, method string, params any) (*JSONRPCMessage, error) {
	var rawID json.RawMessage
	if id != nil {
		b, err := json.Marshal(id)
		if err != nil {
			return nil, err
		}
		rawID = b
	}

	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		rawParams = b
	}

	return &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      rawID,
		Method:  method,
		Params:  rawParams,
	}, nil
}

func NewResponse(id json.RawMessage, result any) (*JSONRPCMessage, error) {
	b, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result:  b,
	}, nil
}

func NewErrorResponse(id json.RawMessage, code int, message string) *JSONRPCMessage {
	return &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
}

func NewNotification(method string, params any) (*JSONRPCMessage, error) {
	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		rawParams = b
	}
	return &JSONRPCMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  rawParams,
	}, nil
}

// IsRequest returns true if this message is a request (has method and id).
func (m *JSONRPCMessage) IsRequest() bool {
	return m.Method != "" && m.ID != nil
}

// IsNotification returns true if this message is a notification (has method but no id).
func (m *JSONRPCMessage) IsNotification() bool {
	return m.Method != "" && m.ID == nil
}

// IsResponse returns true if this message is a response (has id, no method).
func (m *JSONRPCMessage) IsResponse() bool {
	return m.Method == "" && m.ID != nil
}

// MCP Initialize types

type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      Implementation     `json:"clientInfo"`
}

type ClientCapabilities struct {
	Roots    *RootsCapability    `json:"roots,omitempty"`
	Sampling *SamplingCapability `json:"sampling,omitempty"`
}

type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type SamplingCapability struct{}

type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Implementation     `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
}

type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability advertises resource support. Both flags stay false:
// the resource inventory is static, so there is nothing to subscribe to and
// nothing that changes.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// Resource is a concrete, listable resource.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

// ResourceTemplate describes a family of resources whose members cannot be
// enumerated — every dataset on a system, for instance.
type ResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
}

// ResourceContents carries the body of one resource. Text and Blob are
// mutually exclusive; Blob is base64 and used when the content is not valid
// UTF-8.
type ResourceContents struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

type ResourcesListResult struct {
	Resources []Resource `json:"resources"`
}

type ResourceTemplatesListResult struct {
	ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
}

type ResourcesReadParams struct {
	URI string `json:"uri"`
}

type ResourcesReadResult struct {
	Contents []ResourceContents `json:"contents"`
}

type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCP Tool types

// ToolAnnotations carries the MCP behaviour hints a client uses to decide
// whether a tool may run unattended. The hint fields are serialised even
// when false: the MCP spec defaults destructiveHint to true, so an omitted
// false would be read as "destructive".
type ToolAnnotations struct {
	Title           string `json:"title"`
	ReadOnlyHint    bool   `json:"readOnlyHint"`
	DestructiveHint bool   `json:"destructiveHint"`
	IdempotentHint  bool   `json:"idempotentHint"`
	OpenWorldHint   bool   `json:"openWorldHint"`
}

type Tool struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	InputSchema json.RawMessage  `json:"inputSchema"`
	Annotations *ToolAnnotations `json:"annotations,omitempty"`
}

type ToolsListResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type ToolsListParams struct {
	Cursor string `json:"cursor,omitempty"`
}

type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type ToolCallResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

type Content struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Data        string `json:"data,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
	URI         string `json:"uri,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

func TextContent(text string) Content {
	return Content{Type: "text", Text: text}
}

func TextResult(text string) ToolCallResult {
	return ToolCallResult{
		Content: []Content{TextContent(text)},
	}
}

func ErrorResult(text string) ToolCallResult {
	return ToolCallResult{
		Content: []Content{TextContent(text)},
		IsError: true,
	}
}

// ResourceLink returns a pointer to a resource instead of its content. Used
// when a result would otherwise be too large to put in the model's context.
func ResourceLink(uri, name, description, mimeType string, size int64) Content {
	return Content{
		Type:        "resource_link",
		URI:         uri,
		Name:        name,
		Description: description,
		MIMEType:    mimeType,
		Size:        size,
	}
}

// Ping

type PingResult struct{}
