package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestTransport wires a transport to a handler that answers every request
// with a result, so the tests can look at session handling alone.
func newTestTransport(t *testing.T, handler MessageHandler) *HTTPTransport {
	t.Helper()
	tr := NewHTTPTransport(":0", log.New(io.Discard, "", 0), "")
	tr.handler = handler
	return tr
}

func echoHandler(_ context.Context, msg *JSONRPCMessage) *JSONRPCMessage {
	if msg.IsNotification() {
		return nil
	}
	resp, err := NewResponse(msg.ID, map[string]string{"ok": msg.Method})
	if err != nil {
		return NewErrorResponse(msg.ID, ErrCodeInternal, err.Error())
	}
	return resp
}

func post(t *testing.T, tr *HTTPTransport, body, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	rec := httptest.NewRecorder()
	tr.handleMCP(rec, req)
	return rec
}

const initializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`

func TestInitializeHandsOutASessionID(t *testing.T) {
	tr := newTestTransport(t, echoHandler)

	rec := post(t, tr, initializeBody, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	id := rec.Header().Get("Mcp-Session-Id")
	if id == "" {
		t.Fatal("no Mcp-Session-Id header on the initialize response")
	}
	tr.mu.RLock()
	_, exists := tr.sessions[id]
	tr.mu.RUnlock()
	if !exists {
		t.Errorf("session %s was handed out but not stored", id)
	}
}

// A failed initialize must not leave a usable session behind.
func TestFailedInitializeLeavesNoSession(t *testing.T) {
	tr := newTestTransport(t, func(_ context.Context, msg *JSONRPCMessage) *JSONRPCMessage {
		return NewErrorResponse(msg.ID, ErrCodeInvalidParams, "unsupported protocol version")
	})

	rec := post(t, tr, initializeBody, "")

	if id := rec.Header().Get("Mcp-Session-Id"); id != "" {
		t.Errorf("a failed initialize handed out session %s", id)
	}
	tr.mu.RLock()
	n := len(tr.sessions)
	tr.mu.RUnlock()
	if n != 0 {
		t.Errorf("%d session(s) left behind after a failed initialize", n)
	}
}

func TestRequestWithoutSessionIDIsRejected(t *testing.T) {
	tr := newTestTransport(t, echoHandler)

	rec := post(t, tr, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Missing Mcp-Session-Id") {
		t.Errorf("body = %q", rec.Body.String())
	}
}

// After a restart the client still holds its old session id. 404 tells it to
// initialize again instead of retrying forever.
func TestUnknownSessionIDIsNotFound(t *testing.T) {
	tr := newTestTransport(t, echoHandler)

	rec := post(t, tr, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, "00000000-dead-beef-0000-000000000000")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestRequestWithValidSessionReachesTheHandler(t *testing.T) {
	var seen []string
	tr := newTestTransport(t, func(ctx context.Context, msg *JSONRPCMessage) *JSONRPCMessage {
		seen = append(seen, msg.Method)
		return echoHandler(ctx, msg)
	})

	id := post(t, tr, initializeBody, "").Header().Get("Mcp-Session-Id")
	rec := post(t, tr, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, id)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if len(seen) != 2 || seen[1] != "tools/list" {
		t.Errorf("handler saw %v, want initialize then tools/list", seen)
	}
}

func TestInitializedNotificationAdvancesSessionState(t *testing.T) {
	tr := newTestTransport(t, echoHandler)

	id := post(t, tr, initializeBody, "").Header().Get("Mcp-Session-Id")
	rec := post(t, tr, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, id)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	tr.mu.RLock()
	sess := tr.sessions[id]
	tr.mu.RUnlock()
	sess.mu.Lock()
	state := sess.state
	sess.mu.Unlock()
	if state != sessReady {
		t.Errorf("session state = %v, want sessReady", state)
	}
}

// Two clients against the same server must not share state: each initialize
// opens its own session.
func TestSessionsAreIndependent(t *testing.T) {
	tr := newTestTransport(t, echoHandler)

	first := post(t, tr, initializeBody, "").Header().Get("Mcp-Session-Id")
	second := post(t, tr, initializeBody, "").Header().Get("Mcp-Session-Id")

	if first == second {
		t.Fatal("both clients were handed the same session id")
	}
	post(t, tr, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, first)

	tr.mu.RLock()
	s2 := tr.sessions[second]
	tr.mu.RUnlock()
	s2.mu.Lock()
	state := s2.state
	s2.mu.Unlock()
	if state == sessReady {
		t.Error("the second session became ready through the first client's notification")
	}
}

func TestDeleteTerminatesTheSession(t *testing.T) {
	tr := newTestTransport(t, echoHandler)
	id := post(t, tr, initializeBody, "").Header().Get("Mcp-Session-Id")

	req := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	req.Header.Set("Mcp-Session-Id", id)
	rec := httptest.NewRecorder()
	tr.handleMCP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	tr.mu.RLock()
	_, exists := tr.sessions[id]
	tr.mu.RUnlock()
	if exists {
		t.Error("session still present after DELETE")
	}
}

func TestDeleteWithoutSessionIDIsRejected(t *testing.T) {
	tr := newTestTransport(t, echoHandler)

	rec := httptest.NewRecorder()
	tr.handleMCP(rec, httptest.NewRequest(http.MethodDelete, "/mcp", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestUnsupportedMethodIsRejected(t *testing.T) {
	tr := newTestTransport(t, echoHandler)

	rec := httptest.NewRecorder()
	tr.handleMCP(rec, httptest.NewRequest(http.MethodPut, "/mcp", strings.NewReader("{}")))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestEmptyBodyIsRejected(t *testing.T) {
	tr := newTestTransport(t, echoHandler)

	rec := post(t, tr, "", "")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// Malformed JSON is a protocol-level problem, so it comes back as a JSON-RPC
// parse error rather than an HTTP status.
func TestMalformedJSONReturnsAParseError(t *testing.T) {
	tr := newTestTransport(t, echoHandler)

	rec := post(t, tr, `{"jsonrpc":`, "")

	var resp JSONRPCMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != ErrCodeParse {
		t.Errorf("error = %+v, want code %d", resp.Error, ErrCodeParse)
	}
}

func TestBatchRequestReturnsAnArray(t *testing.T) {
	tr := newTestTransport(t, echoHandler)

	rec := post(t, tr, `[{"jsonrpc":"2.0","id":1,"method":"tools/list"},{"jsonrpc":"2.0","id":2,"method":"resources/list"}]`, "")

	var responses []JSONRPCMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &responses); err != nil {
		t.Fatalf("response is not a JSON array: %v (%s)", err, rec.Body.String())
	}
	if len(responses) != 2 {
		t.Errorf("got %d responses, want 2", len(responses))
	}
}
