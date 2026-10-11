package zosmf

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func tsoMsg(s string) map[string]any {
	return map[string]any{"TSO MESSAGE": map[string]string{"VERSION": "0100", "DATA": s}}
}

func tsoPrompt() map[string]any {
	return map[string]any{"TSO PROMPT": map[string]string{"VERSION": "0100", "HIDDEN": "FALSE"}}
}

// fakeTSO plays z/OSMF's TSO/E address space service: POST starts an address
// space, GET returns the next queued batch of TSO data, PUT sends a command,
// DELETE stops the address space.
type fakeTSO struct {
	mu         sync.Mutex
	start      map[string]any     // body of the POST response
	polls      [][]map[string]any // successive GET responses
	putStatus  int
	startQuery url.Values
	putBody    string
	calls      []string
	deleted    bool
}

func (f *fakeTSO) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/zosmf/tsoApp/tso":
			f.startQuery = r.URL.Query()
			if b, _ := io.ReadAll(r.Body); len(b) > 0 {
				t.Errorf("start sent a body: %s", b)
			}
			reply(f.start)
		case r.Method == http.MethodGet && r.URL.Path == "/zosmf/tsoApp/tso/KEY1":
			var data []map[string]any
			if len(f.polls) > 0 {
				data, f.polls = f.polls[0], f.polls[1:]
			}
			reply(map[string]any{"servletKey": "KEY1", "tsoData": data})
		case r.Method == http.MethodPut && r.URL.Path == "/zosmf/tsoApp/tso/KEY1":
			if r.URL.Query().Get("readReply") != "false" {
				t.Errorf("PUT without readReply=false: %s", r.URL.RawQuery)
			}
			b, _ := io.ReadAll(r.Body)
			f.putBody = string(b)
			if f.putStatus != 0 {
				w.WriteHeader(f.putStatus)
				return
			}
			reply(map[string]any{"servletKey": "KEY1", "tsoData": []any{}})
		case r.Method == http.MethodDelete && r.URL.Path == "/zosmf/tsoApp/tso/KEY1":
			f.deleted = true
			reply(map[string]any{"servletKey": "KEY1"})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func fastTSOPolling(t *testing.T, timeout time.Duration) {
	t.Helper()
	oldInterval, oldTimeout := tsoPollInterval, tsoTimeout
	tsoPollInterval, tsoTimeout = time.Millisecond, timeout
	t.Cleanup(func() { tsoPollInterval, tsoTimeout = oldInterval, oldTimeout })
}

// With an account, the command runs in an address space started with exactly
// the configured settings, and only the command's own output comes back.
func TestRunTSOCommandWithAccountUsesTheConfiguredAddressSpace(t *testing.T) {
	fastTSOPolling(t, time.Second)
	f := &fakeTSO{
		start: map[string]any{"servletKey": "KEY1", "tsoData": []any{tsoMsg("IKJ56455I IBMUSER LOGON IN PROGRESS")}},
		polls: [][]map[string]any{
			{tsoMsg("READY"), tsoPrompt()}, // end of logon
			{tsoPrompt()},                  // a prompt before any output belongs to the previous exchange
			{tsoMsg("IKJ56650I TIME-10:15:02 AM")},
			{tsoMsg("READY"), tsoPrompt()},
		},
	}
	c := testClient(t, f.handler(t))

	out, err := c.RunTSOCommand("TIME", TSOAddressSpace{Account: "ACCT#", Proc: "MYPROC", Charset: "1140", Codepage: "500"})
	if err != nil {
		t.Fatalf("RunTSOCommand: %v", err)
	}
	if out != "IKJ56650I TIME-10:15:02 AM\nREADY" {
		t.Errorf("output = %q, want the command's output only", out)
	}
	for key, want := range map[string]string{"acct": "ACCT#", "proc": "MYPROC", "chset": "1140", "cpage": "500"} {
		if got := f.startQuery.Get(key); got != want {
			t.Errorf("start %s = %q, want %q", key, got, want)
		}
	}
	if !strings.Contains(f.putBody, `"TSO RESPONSE"`) || !strings.Contains(f.putBody, `"DATA":"TIME"`) {
		t.Errorf("command body = %s", f.putBody)
	}
	if !f.deleted {
		t.Error("the address space was not stopped")
	}
}

func TestRunTSOCommandWithAccountFillsTheDefaults(t *testing.T) {
	fastTSOPolling(t, time.Second)
	f := &fakeTSO{
		start: map[string]any{"servletKey": "KEY1", "tsoData": []any{tsoMsg("READY"), tsoPrompt()}},
		polls: [][]map[string]any{{tsoMsg("READY"), tsoPrompt()}},
	}
	c := testClient(t, f.handler(t))

	if _, err := c.RunTSOCommand("TIME", TSOAddressSpace{Account: "ACCT1"}); err != nil {
		t.Fatalf("RunTSOCommand: %v", err)
	}
	for key, want := range map[string]string{"proc": "IZUFPROC", "chset": "697", "cpage": "1047"} {
		if got := f.startQuery.Get(key); got != want {
			t.Errorf("start %s = %q, want the default %q", key, got, want)
		}
	}
}

// A logon procedure that does not exist must fail the call, not run the
// command somewhere else — that is the point of configuring one.
func TestRunTSOCommandReportsAnInvalidLogonProcedure(t *testing.T) {
	fastTSOPolling(t, time.Second)
	f := &fakeTSO{
		start: map[string]any{"servletKey": "KEY1", "tsoData": []any{
			tsoMsg("IKJ56482I THE PROCEDURE NAME NOSUCHPR IS NOT DEFINED FOR YOUR USERID"),
			tsoPrompt(),
		}},
	}
	c := testClient(t, f.handler(t))

	_, err := c.RunTSOCommand("TIME", TSOAddressSpace{Account: "ACCT1", Proc: "NOSUCHPR"})
	if err == nil || !strings.Contains(err.Error(), "IKJ56482I") {
		t.Fatalf("err = %v, want the IKJ56482I message", err)
	}
	if f.putBody != "" {
		t.Error("the command was sent despite the failed logon")
	}
	if !f.deleted {
		t.Error("the address space was not stopped")
	}
}

func TestRunTSOCommandReportsAZosmfErrorOnStart(t *testing.T) {
	fastTSOPolling(t, time.Second)
	f := &fakeTSO{
		start: map[string]any{"msgData": []any{map[string]string{"messageText": "IZUG1126E: The TSO/E address space could not be started."}}},
	}
	c := testClient(t, f.handler(t))

	_, err := c.RunTSOCommand("TIME", TSOAddressSpace{Account: "ACCT1"})
	if err == nil || !strings.Contains(err.Error(), "IZUG1126E") {
		t.Fatalf("err = %v, want the z/OSMF message", err)
	}
}

func TestRunTSOCommandStopsTheAddressSpaceWhenSendingFails(t *testing.T) {
	fastTSOPolling(t, time.Second)
	f := &fakeTSO{
		start:     map[string]any{"servletKey": "KEY1", "tsoData": []any{tsoMsg("READY"), tsoPrompt()}},
		putStatus: http.StatusInternalServerError,
	}
	c := testClient(t, f.handler(t))

	if _, err := c.RunTSOCommand("TIME", TSOAddressSpace{Account: "ACCT1"}); err == nil {
		t.Fatal("expected an error from the failed send")
	}
	if !f.deleted {
		t.Error("the address space was not stopped after the failed send")
	}
}

func TestRunTSOCommandGivesUpWhenNoPromptComes(t *testing.T) {
	fastTSOPolling(t, 50*time.Millisecond)
	f := &fakeTSO{
		start: map[string]any{"servletKey": "KEY1", "tsoData": []any{tsoMsg("READY"), tsoPrompt()}},
	}
	c := testClient(t, f.handler(t))

	_, err := c.RunTSOCommand("TIME", TSOAddressSpace{Account: "ACCT1"})
	if err == nil || !strings.Contains(err.Error(), "no TSO prompt") {
		t.Fatalf("err = %v, want a timeout naming the missing prompt", err)
	}
	if !f.deleted {
		t.Error("the address space was not stopped after the timeout")
	}
}

// Without an account, the single-request service runs the command and z/OSMF
// chooses the address space settings itself.
func TestRunTSOCommandWithoutAccountUsesTheSingleRequestService(t *testing.T) {
	var body string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/zosmf/tsoApp/v1/tso" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		_, _ = io.WriteString(w, `{"cmdResponse":[{"message":"IKJ56650I TIME-10:15:02 AM"},{"message":"READY"}]}`)
	})

	out, err := c.RunTSOCommand("TIME", TSOAddressSpace{})
	if err != nil {
		t.Fatalf("RunTSOCommand: %v", err)
	}
	if out != "IKJ56650I TIME-10:15:02 AM\nREADY" {
		t.Errorf("output = %q", out)
	}
	if body != `{"tsoCmd":"TIME"}` {
		t.Errorf("body = %s", body)
	}
}
