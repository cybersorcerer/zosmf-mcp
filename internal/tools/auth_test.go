package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/config"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

func TestSetPasswordTool_UpdatesClientPassword(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	client, err := zosmf.NewClient(config.SystemConfig{
		Name: "SYSA",
		Host: srv.URL,
		Auth: config.AuthConfig{Type: "basic", Username: "user1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := zosmf.NewClientRegistry()
	registry.Register("SYSA", client)

	tool := NewSetPasswordTool(registry)
	args, _ := json.Marshal(map[string]string{"system": "SYSA", "password": "geheim"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned error result: %+v", result.Content)
	}
	if strings.Contains(result.Content[0].Text, "geheim") {
		t.Error("result text must not contain the password")
	}

	if _, err := client.Get("/zosmf/info"); err != nil {
		t.Fatalf("Get after set_password: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user1:geheim"))
	if capturedAuth != want {
		t.Errorf("Authorization = %q, want %q", capturedAuth, want)
	}
}

func TestSetPasswordTool_UnknownSystem_ReturnsErrorResult(t *testing.T) {
	tool := NewSetPasswordTool(zosmf.NewClientRegistry())
	args, _ := json.Marshal(map[string]string{"system": "NOPE", "password": "x"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for unknown system")
	}
	if !strings.Contains(result.Content[0].Text, "unknown system") {
		t.Errorf("unexpected error text: %q", result.Content[0].Text)
	}
}

func TestSetPasswordTool_MissingArgs_ReturnsErrorResult(t *testing.T) {
	tool := NewSetPasswordTool(zosmf.NewClientRegistry())
	args, _ := json.Marshal(map[string]string{"system": "SYSA"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing password")
	}
}
