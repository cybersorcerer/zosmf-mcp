package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// StdioTransport implements the MCP stdio transport.
// It reads newline-delimited JSON from stdin and writes responses to stdout.
type StdioTransport struct {
	reader io.Reader
	writer io.Writer
	logger *log.Logger
	mu     sync.Mutex // protects writer
}

func NewStdioTransport(logger *log.Logger) *StdioTransport {
	return &StdioTransport{
		reader: os.Stdin,
		writer: os.Stdout,
		logger: logger,
	}
}

// NewStdioTransportWithIO creates a stdio transport with custom reader/writer (for testing).
func NewStdioTransportWithIO(reader io.Reader, writer io.Writer, logger *log.Logger) *StdioTransport {
	return &StdioTransport{
		reader: reader,
		writer: writer,
		logger: logger,
	}
}

func (t *StdioTransport) Run(ctx context.Context, handler MessageHandler) error {
	scanner := bufio.NewScanner(t.reader)
	// Allow up to 10MB per line for large spool content
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Try to parse as a batch (array) first
		if line[0] == '[' {
			t.handleBatch(ctx, line, handler)
			continue
		}

		// Single message
		var msg JSONRPCMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			t.logger.Printf("failed to parse message: %v", err)
			errResp := NewErrorResponse(nil, ErrCodeParse, "parse error")
			t.writeMessage(errResp)
			continue
		}

		resp := handler(ctx, &msg)
		if resp != nil {
			t.writeMessage(resp)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stdin read error: %w", err)
	}

	// EOF on stdin — clean shutdown
	t.logger.Println("stdin closed, shutting down")
	return nil
}

func (t *StdioTransport) handleBatch(ctx context.Context, data []byte, handler MessageHandler) {
	var batch []JSONRPCMessage
	if err := json.Unmarshal(data, &batch); err != nil {
		t.logger.Printf("failed to parse batch: %v", err)
		errResp := NewErrorResponse(nil, ErrCodeParse, "parse error")
		t.writeMessage(errResp)
		return
	}

	var responses []*JSONRPCMessage
	for i := range batch {
		resp := handler(ctx, &batch[i])
		if resp != nil {
			responses = append(responses, resp)
		}
	}

	if len(responses) > 0 {
		t.writeBatch(responses)
	}
}

func (t *StdioTransport) writeMessage(msg *JSONRPCMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		t.logger.Printf("failed to marshal response: %v", err)
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = fmt.Fprintf(t.writer, "%s\n", data)
}

func (t *StdioTransport) writeBatch(msgs []*JSONRPCMessage) {
	data, err := json.Marshal(msgs)
	if err != nil {
		t.logger.Printf("failed to marshal batch response: %v", err)
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = fmt.Fprintf(t.writer, "%s\n", data)
}
