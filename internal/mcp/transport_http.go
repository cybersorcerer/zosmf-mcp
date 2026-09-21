package mcp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// HTTPTransport implements the MCP Streamable HTTP transport.
type HTTPTransport struct {
	addr    string
	logger  *log.Logger
	apiKey  string
	handler MessageHandler

	mu       sync.RWMutex
	sessions map[string]*session
}

type sessionState int

const (
	sessUninitialized sessionState = iota
	sessInitializing
	sessReady
)

type session struct {
	id        string
	createdAt time.Time
	sseCh     chan *JSONRPCMessage // channel for server-initiated messages
	mu        sync.Mutex
	state     sessionState
}

func NewHTTPTransport(addr string, logger *log.Logger, apiKey string) *HTTPTransport {
	return &HTTPTransport{
		addr:     addr,
		logger:   logger,
		apiKey:   apiKey,
		sessions: make(map[string]*session),
	}
}

// requireAPIKey rejects requests without a matching bearer token. An empty key
// disables the check, which keeps stdio-only setups unaffected.
//
// Rejections are logged. Without that line an operator sees nothing at all for
// a request that never reached a handler, which makes a wrong key on the client
// side indistinguishable from the server being unreachable. The log records
// only whether a token was present, never the token itself.
func requireAPIKey(key string, logger *log.Logger, next http.Handler) http.Handler {
	if key == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
			reason := "no bearer token"
			if ok {
				reason = "wrong bearer token"
			}
			logger.Printf("rejected %s %s: %s", r.Method, r.URL.Path, reason)
			w.Header().Set("WWW-Authenticate", `Bearer realm="zosmf-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (t *HTTPTransport) Run(ctx context.Context, handler MessageHandler) error {
	t.handler = handler

	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", t.handleMCP)

	server := &http.Server{
		Addr:    t.addr,
		Handler: requireAPIKey(t.apiKey, t.logger, mux),
	}

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (t *HTTPTransport) handleMCP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		t.handlePost(w, r)
	case http.MethodGet:
		t.handleGet(w, r)
	case http.MethodDelete:
		t.handleDelete(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (t *HTTPTransport) handlePost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	if len(body) == 0 {
		http.Error(w, "Empty body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Try batch first
	if body[0] == '[' {
		t.handleBatchPost(ctx, w, r, body)
		return
	}

	var msg JSONRPCMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		resp := NewErrorResponse(nil, ErrCodeParse, "parse error")
		writeJSON(w, resp)
		return
	}

	if msg.Method == "initialize" {
		// Each initialize creates a fresh session with its own state
		sess := t.createSession()
		sess.mu.Lock()
		sess.state = sessInitializing
		sess.mu.Unlock()
		resp := t.handler(ctx, &msg)
		if resp != nil && resp.Error == nil {
			w.Header().Set("Mcp-Session-Id", sess.id)
		} else {
			// initialization failed — remove the session
			t.mu.Lock()
			delete(t.sessions, sess.id)
			t.mu.Unlock()
		}
		if resp != nil {
			writeJSON(w, resp)
		} else {
			w.WriteHeader(http.StatusAccepted)
		}
		return
	}

	// Non-initialize: require a valid session
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		http.Error(w, "Missing Mcp-Session-Id header", http.StatusBadRequest)
		return
	}
	t.mu.RLock()
	sess, exists := t.sessions[sessionID]
	t.mu.RUnlock()
	if !exists {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Handle notifications/initialized per session
	if msg.IsNotification() && msg.Method == "notifications/initialized" {
		sess.mu.Lock()
		if sess.state == sessInitializing {
			sess.state = sessReady
		}
		sess.mu.Unlock()
		t.handler(ctx, &msg)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	resp := t.handler(ctx, &msg)

	if resp == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	writeJSON(w, resp)
}

func (t *HTTPTransport) handleBatchPost(ctx context.Context, w http.ResponseWriter, r *http.Request, body []byte) {
	var batch []JSONRPCMessage
	if err := json.Unmarshal(body, &batch); err != nil {
		resp := NewErrorResponse(nil, ErrCodeParse, "parse error")
		writeJSON(w, resp)
		return
	}

	var responses []*JSONRPCMessage
	for i := range batch {
		resp := t.handler(ctx, &batch[i])
		if resp != nil {
			responses = append(responses, resp)
		}
		// If this batch contains an initialize request, create a session
		if batch[i].Method == "initialize" && resp != nil && resp.Error == nil {
			sess := t.createSession()
			w.Header().Set("Mcp-Session-Id", sess.id)
		}
	}

	if len(responses) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(responses)
}

func (t *HTTPTransport) handleGet(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		http.Error(w, "Missing Mcp-Session-Id header", http.StatusBadRequest)
		return
	}

	t.mu.RLock()
	sess, exists := t.sessions[sessionID]
	t.mu.RUnlock()
	if !exists {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Check if client accepts SSE
	accept := r.Header.Get("Accept")
	if accept != "text/event-stream" {
		http.Error(w, "Accept must be text/event-stream", http.StatusNotAcceptable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-sess.sseCh:
			if !ok {
				return
			}
			data, err := json.Marshal(msg)
			if err != nil {
				t.logger.Printf("failed to marshal SSE message: %v", err)
				continue
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (t *HTTPTransport) handleDelete(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		http.Error(w, "Missing Mcp-Session-Id header", http.StatusBadRequest)
		return
	}

	t.mu.Lock()
	sess, exists := t.sessions[sessionID]
	if exists {
		close(sess.sseCh)
		delete(t.sessions, sessionID)
	}
	t.mu.Unlock()

	t.logger.Printf("session %s terminated", sessionID)
	w.WriteHeader(http.StatusOK)
}

func (t *HTTPTransport) createSession() *session {
	sess := &session{
		id:        uuid.New().String(),
		createdAt: time.Now(),
		sseCh:     make(chan *JSONRPCMessage, 16),
	}

	t.mu.Lock()
	t.sessions[sess.id] = sess
	t.mu.Unlock()

	t.logger.Printf("created session: %s", sess.id)
	return sess
}

func writeJSON(w http.ResponseWriter, msg *JSONRPCMessage) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(msg)
}
