package mcp

import (
	"encoding/json"
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
