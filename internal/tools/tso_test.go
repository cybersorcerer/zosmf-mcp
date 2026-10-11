package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
)

func runTSOTool(t *testing.T, sys config.SystemConfig, handler http.HandlerFunc) (string, bool) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	sys.Name = "S0W1"
	tool := NewTSOCommandTool(newToolTestClients(t, srv.URL), &config.Config{Systems: []config.SystemConfig{sys}})
	args, _ := json.Marshal(map[string]any{"system": "S0W1", "command": "TIME"})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var text string
	for _, c := range result.Content {
		text += c.Text
	}
	return text, result.IsError
}

// Without tso_account the command still runs — z/OSMF picks the address
// space settings itself — instead of being refused.
func TestTSOCommandWithoutAccountRuns(t *testing.T) {
	out, isErr := runTSOTool(t, config.SystemConfig{}, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zosmf/tsoApp/v1/tso" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		_, _ = io.WriteString(w, `{"cmdResponse":[{"message":"IKJ56650I TIME-10:15:02 AM"},{"message":"READY"}]}`)
	})
	if isErr || !strings.Contains(out, "IKJ56650I") {
		t.Errorf("isError=%v, output %q", isErr, out)
	}
}

// The configured account and procedure must reach z/OSMF.
func TestTSOCommandWithAccountPassesTheConfiguration(t *testing.T) {
	var query string
	out, isErr := runTSOTool(t, config.SystemConfig{TSOAccount: "ACCT#", TSOProc: "MYPROC"}, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			query = r.URL.RawQuery
			_, _ = io.WriteString(w, `{"servletKey":"K","tsoData":[{"TSO MESSAGE":{"DATA":"READY"}},{"TSO PROMPT":{"HIDDEN":"FALSE"}}]}`)
		case http.MethodPut:
			_, _ = io.WriteString(w, `{"servletKey":"K","tsoData":[{"TSO MESSAGE":{"DATA":"IKJ56650I TIME-10:15:02 AM"}},{"TSO MESSAGE":{"DATA":"READY"}},{"TSO PROMPT":{"HIDDEN":"FALSE"}}]}`)
		case http.MethodDelete:
			_, _ = io.WriteString(w, `{"servletKey":"K"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	})
	if isErr || !strings.Contains(out, "IKJ56650I") {
		t.Errorf("isError=%v, output %q", isErr, out)
	}
	if !strings.Contains(query, "acct=ACCT%23") || !strings.Contains(query, "proc=MYPROC") {
		t.Errorf("start query = %q, want the configured account and procedure", query)
	}
}

// A procedure, character set or code page without an account cannot be
// honored — the single-request service takes none of them. Running anyway
// would silently ignore them again.
func TestTSOCommandRefusesSettingsWithoutAnAccount(t *testing.T) {
	out, isErr := runTSOTool(t, config.SystemConfig{TSOProc: "MYPROC"}, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request may be sent, got %s %s", r.Method, r.URL)
	})
	if !isErr || !strings.Contains(out, "tso_account") || !strings.Contains(out, "tso_proc") {
		t.Errorf("isError=%v, output %q; want an error naming tso_account and tso_proc", isErr, out)
	}
}
