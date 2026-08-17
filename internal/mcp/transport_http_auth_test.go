package mcp

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func discardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("reached"))
	})
}

func TestRequireAPIKeyRejectsMissingHeader(t *testing.T) {
	h := requireAPIKey("s3cret", discardLogger(), okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got == "" {
		t.Error("expected WWW-Authenticate header")
	}
	if rec.Body.String() == "reached" {
		t.Error("handler must not be reached")
	}
}

func TestRequireAPIKeyRejectsWrongKey(t *testing.T) {
	h := requireAPIKey("s3cret", discardLogger(), okHandler())
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestRequireAPIKeyAcceptsCorrectKey(t *testing.T) {
	h := requireAPIKey("s3cret", discardLogger(), okHandler())
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer s3cret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "reached" {
		t.Error("handler should have been reached")
	}
}

func TestRequireAPIKeyDisabledWhenEmpty(t *testing.T) {
	h := requireAPIKey("", discardLogger(), okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 when no key configured", rec.Code)
	}
}

func TestRequireAPIKeyRejectsBareKeyWithoutBearer(t *testing.T) {
	h := requireAPIKey("s3cret", discardLogger(), okHandler())
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "s3cret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 — only the Bearer scheme is accepted", rec.Code)
	}
}

func TestRequireAPIKeyLogsRejections(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		wantReason string
	}{
		{"missing header", "", "no bearer token"},
		{"wrong token", "Bearer wrong-key-value", "wrong bearer token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logged bytes.Buffer
			h := requireAPIKey("s3cret", log.New(&logged, "", 0), okHandler())

			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)

			out := logged.String()
			if out == "" {
				t.Fatal("rejection was not logged — an operator would see nothing at all")
			}
			if !strings.Contains(out, tt.wantReason) {
				t.Errorf("log = %q, want it to name the reason %q", out, tt.wantReason)
			}
			if strings.Contains(out, "s3cret") {
				t.Error("log leaks the configured API key")
			}
			if strings.Contains(out, "wrong-key-value") {
				t.Error("log leaks the token the client sent")
			}
			if strings.Contains(out, "192.0.2.1") || strings.Contains(out, req.RemoteAddr) {
				t.Error("log records the client address, which it must not")
			}
		})
	}
}

func TestRequireAPIKeyLogsNothingOnSuccess(t *testing.T) {
	var logged bytes.Buffer
	h := requireAPIKey("s3cret", log.New(&logged, "", 0), okHandler())

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer s3cret")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if logged.String() != "" {
		t.Errorf("accepted request should not log, got %q", logged.String())
	}
}
