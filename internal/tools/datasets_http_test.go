package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/zosmf"
)

// registryAgainst registers one system named MAINT whose client talks to the
// given handler, so a tool can be driven end to end over real HTTP.
func registryAgainst(t *testing.T, handler http.HandlerFunc) *zosmf.ClientRegistry {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client, err := zosmf.NewClient(config.SystemConfig{
		Name: "MAINT",
		Host: srv.URL,
		Auth: config.AuthConfig{Type: "basic", Username: "ibmuser", Password: "secret"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	reg := zosmf.NewClientRegistry()
	reg.Register("MAINT", client)
	return reg
}

func TestListDatasetsReturnsTheItems(t *testing.T) {
	var gotQuery string
	reg := registryAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"items":[{"dsname":"SYS1.PARMLIB","dsorg":"PO","vol":"Z31RS2"}],"returnedRows":1}`))
	})

	res := execTool(t, NewListDatasetsTool(reg), `{"system":"MAINT","dslevel":"SYS1.PARM*"}`)

	if res.IsError {
		t.Fatalf("unexpected error result: %s", resultText(res))
	}
	if !strings.Contains(gotQuery, "SYS1.PARM") {
		t.Errorf("query = %q, want the dslevel passed through", gotQuery)
	}
	var datasets []map[string]any
	if err := json.Unmarshal([]byte(resultText(res)), &datasets); err != nil {
		t.Fatalf("result is not a JSON array: %v (%s)", err, resultText(res))
	}
	if len(datasets) != 1 || datasets[0]["dsname"] != "SYS1.PARMLIB" {
		t.Errorf("datasets = %v", datasets)
	}
}

// A wrong password must stay recognisable all the way up to the tool result,
// otherwise the client cannot tell it from a missing dataset.
func TestListDatasetsPropagatesAuthFailure(t *testing.T) {
	reg := registryAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"authentication failed"}`))
	})

	res := execTool(t, NewListDatasetsTool(reg), `{"system":"MAINT","dslevel":"SYS1.*"}`)

	if !res.IsError {
		t.Fatal("a 401 must produce an error result")
	}
	if !strings.Contains(resultText(res), "AUTH_FAILED(MAINT)") {
		t.Errorf("result = %q, want the AUTH_FAILED marker", resultText(res))
	}
}

func TestListDatasetsRejectsUnknownSystem(t *testing.T) {
	reg := registryAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("the request must not reach z/OSMF")
	})

	res := execTool(t, NewListDatasetsTool(reg), `{"system":"NOPE","dslevel":"SYS1.*"}`)

	if !res.IsError || !strings.Contains(resultText(res), "unknown system: NOPE") {
		t.Errorf("result = %q, want an unknown-system error", resultText(res))
	}
}

func TestListMembersSurfacesTheZOSMFStatus(t *testing.T) {
	reg := registryAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"data set not found"}`))
	})

	res := execTool(t, NewListMembersTool(reg), `{"system":"MAINT","dataset":"NO.SUCH.PDS"}`)

	if !res.IsError {
		t.Fatal("a 404 must produce an error result")
	}
	for _, want := range []string{"failed to list members", "data set not found"} {
		if !strings.Contains(resultText(res), want) {
			t.Errorf("result %q lacks %q", resultText(res), want)
		}
	}
}

func TestListDatasetsRejectsMalformedArguments(t *testing.T) {
	reg := registryAgainst(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("the request must not reach z/OSMF")
	})

	res := execTool(t, NewListDatasetsTool(reg), `{"system":`)

	if !res.IsError || !strings.Contains(resultText(res), "invalid arguments") {
		t.Errorf("result = %q", resultText(res))
	}
}
