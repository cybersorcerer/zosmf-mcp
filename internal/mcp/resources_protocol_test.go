package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"testing"
)

func TestInitializeAdvertisesResources(t *testing.T) {
	s := NewServer(NewToolRegistry(), log.New(os.Stderr, "", 0))

	params, _ := json.Marshal(InitializeParams{
		ProtocolVersion: "2025-06-18",
		ClientInfo:      Implementation{Name: "test", Version: "1"},
	})
	msg := &JSONRPCMessage{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "initialize", Params: params}

	resp := s.HandleMessage(nil, msg)
	if resp.Error != nil {
		t.Fatalf("initialize failed: %v", resp.Error)
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ProtocolVersion != "2025-06-18" {
		t.Errorf("ProtocolVersion = %q, want 2025-06-18", result.ProtocolVersion)
	}
	if result.Capabilities.Resources == nil {
		t.Fatal("Capabilities.Resources is nil, want advertised")
	}
	if result.Capabilities.Resources.Subscribe {
		t.Error("Subscribe = true, want false")
	}
}

func TestInitializeAcceptsOlderProtocolVersion(t *testing.T) {
	s := NewServer(NewToolRegistry(), log.New(os.Stderr, "", 0))

	params, _ := json.Marshal(InitializeParams{
		ProtocolVersion: "2025-03-26",
		ClientInfo:      Implementation{Name: "old-client", Version: "1"},
	})
	msg := &JSONRPCMessage{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "initialize", Params: params}

	resp := s.HandleMessage(nil, msg)
	if resp.Error != nil {
		t.Fatalf("initialize with older protocol version failed: %v", resp.Error)
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ProtocolVersion != "2025-06-18" {
		t.Errorf("ProtocolVersion = %q, want the version the server speaks", result.ProtocolVersion)
	}
}

func TestResourceLinkMarshalling(t *testing.T) {
	c := ResourceLink("zosmf://S0W1/dataset/USER.DATA", "USER.DATA", "sequential dataset", "text/plain", 4096)

	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["type"] != "resource_link" {
		t.Errorf("type = %v, want resource_link", got["type"])
	}
	if got["uri"] != "zosmf://S0W1/dataset/USER.DATA" {
		t.Errorf("uri = %v", got["uri"])
	}
	if got["size"] != float64(4096) {
		t.Errorf("size = %v, want 4096", got["size"])
	}
	if _, present := got["text"]; present {
		t.Error("text must be omitted on a resource_link")
	}
}

func TestResourceMethodsWithoutProvider(t *testing.T) {
	for _, method := range []string{"resources/list", "resources/templates/list", "resources/read"} {
		t.Run(method, func(t *testing.T) {
			s := NewHTTPServer(NewToolRegistry(), log.New(os.Stderr, "", 0))

			msg := &JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`1`),
				Method:  method,
				Params:  json.RawMessage(`{"uri":"zosmf:///systems"}`),
			}

			resp := s.HandleMessage(context.Background(), msg)

			if resp.Error == nil {
				t.Fatalf("%s without a provider returned no error, want method-not-found", method)
			}
			if resp.Error.Code != ErrCodeMethodNotFound {
				t.Errorf("code = %d, want %d", resp.Error.Code, ErrCodeMethodNotFound)
			}
		})
	}
}

// stubProvider returns whatever error the test asks for, so the mapping from
// error kind to JSON-RPC code can be checked without a resource registry.
type stubProvider struct{ err error }

func (p stubProvider) List() []Resource              { return nil }
func (p stubProvider) Templates() []ResourceTemplate { return nil }
func (p stubProvider) Read(context.Context, string) (ResourceContents, error) {
	return ResourceContents{}, p.err
}

func TestResourcesReadErrorCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"malformed URI", fmt.Errorf("%w: garbage", ErrInvalidResourceURI), ErrCodeInvalidParams},
		{"unknown resource", fmt.Errorf("%w: zosmf://S0W1/dataset/NO.SUCH", ErrResourceNotFound), ErrCodeResourceNotFound},
		{"SAF denial", errors.New("access to dataset PROD.PAYROLL denied by SAF/RACF"), ErrCodeInternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewHTTPServer(NewToolRegistry(), log.New(os.Stderr, "", 0))
			s.SetResourceProvider(stubProvider{err: tt.err})

			msg := &JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`1`),
				Method:  "resources/read",
				Params:  json.RawMessage(`{"uri":"zosmf://S0W1/dataset/PROD.PAYROLL"}`),
			}

			resp := s.HandleMessage(context.Background(), msg)
			if resp.Error == nil {
				t.Fatal("no error returned")
			}
			if resp.Error.Code != tt.want {
				t.Errorf("code = %d, want %d", resp.Error.Code, tt.want)
			}
		})
	}
}
