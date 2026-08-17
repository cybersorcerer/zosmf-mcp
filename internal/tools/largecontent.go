package tools

import (
	"strings"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
)

// DefaultInlineMaxBytes is how much content a tool result may carry as inline
// text before it switches to a preview plus a resource_link. Roughly eight to
// ten thousand tokens — enough for JESYSMSG and a typical PARMLIB member, too
// little for the SYSPRINT of a compile.
const DefaultInlineMaxBytes = 32768

// previewBytes is how much of an oversized body still goes inline. A
// constant, not a configuration value: a second knob for something nobody
// wants to tune.
const previewBytes = 2048

// preview returns the head of text, cut at the last line boundary that fits
// so the result does not end mid-record.
func preview(text string) string {
	if len(text) <= previewBytes {
		return text
	}
	head := text[:previewBytes]
	if i := strings.LastIndexByte(head, '\n'); i > 0 {
		return head[:i+1]
	}
	return head
}

// textOrLink returns the body as inline text when it fits within max, and a
// preview plus the given resource_link when it does not. The preview goes out
// even to clients that support resources: it costs little and means a result
// is never a bare pointer.
func textOrLink(text string, max int, link mcp.Content) []mcp.Content {
	if len(text) <= max {
		return []mcp.Content{mcp.TextContent(text)}
	}
	return []mcp.Content{mcp.TextContent(preview(text)), link}
}
