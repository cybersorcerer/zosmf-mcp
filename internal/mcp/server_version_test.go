package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"testing"
)

// The version a client reads from initialize used to be a constant that was
// last touched at 0.3.0, so every release after that reported the wrong one.
// It is now the version the binary was built with.
func TestInitializeReportsTheBuildVersion(t *testing.T) {
	previous := ServerVersion
	t.Cleanup(func() { ServerVersion = previous })
	ServerVersion = "9.9.9-test"

	s := NewServer(NewToolRegistry(), log.New(io.Discard, "", 0))
	msg := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe","version":"1"}}`),
	}

	resp := s.HandleMessage(context.Background(), msg)
	if resp == nil || resp.Error != nil {
		t.Fatalf("initialize failed: %+v", resp)
	}

	var result InitializeResult
	data, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshalling the result: %v", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshalling the result: %v", err)
	}

	if result.ServerInfo.Version != "9.9.9-test" {
		t.Errorf("serverInfo.version = %q, want the build version", result.ServerInfo.Version)
	}
	if result.ServerInfo.Name != ServerName {
		t.Errorf("serverInfo.name = %q, want %q", result.ServerInfo.Name, ServerName)
	}
}

// Built without ldflags the version is unknown rather than a stale number.
func TestServerVersionDefaultsToDev(t *testing.T) {
	if ServerVersion != "dev" {
		t.Errorf("default ServerVersion = %q, want dev — the release version is injected at build time", ServerVersion)
	}
}
