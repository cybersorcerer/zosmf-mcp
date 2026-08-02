package mcp

import (
	"encoding/json"
	"strings"
)

// maxLoggedArgsLen caps how much of a tool call's arguments end up in the log.
const maxLoggedArgsLen = 512

// sensitiveKeys are argument names whose values must never be logged.
var sensitiveKeys = []string{"password", "passwd", "secret", "token", "passphrase"}

// formatArgs renders tool call arguments for logging: secrets are redacted and
// the result is truncated to keep log lines readable.
func formatArgs(args json.RawMessage) string {
	if len(args) == 0 {
		return "{}"
	}

	out := string(args)

	var m map[string]any
	if err := json.Unmarshal(args, &m); err == nil {
		redactMap(m)
		if b, err := json.Marshal(m); err == nil {
			out = string(b)
		}
	}

	if len(out) > maxLoggedArgsLen {
		out = out[:maxLoggedArgsLen] + "…(truncated)"
	}
	return out
}

func redactMap(m map[string]any) {
	for k, v := range m {
		if isSensitiveKey(k) {
			m[k] = "***"
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			redactMap(nested)
		}
	}
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// resultSize returns the total length of all text/data content in a result.
func resultSize(result *ToolCallResult) int {
	if result == nil {
		return 0
	}
	n := 0
	for _, c := range result.Content {
		n += len(c.Text) + len(c.Data)
	}
	return n
}
