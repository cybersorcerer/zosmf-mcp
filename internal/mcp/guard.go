package mcp

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Every byte a tool or resource returns ends up in the model's context and is
// re-sent with every later request of the conversation. A single oversized or
// garbled result can therefore break the whole session on the client side
// (seen with Claude Code: "400 The request body is not valid JSON"). The guard
// below is the last line of defence, applied centrally to every tools/call and
// resources/read response, independent of what the individual tool does.

// MaxResultTextBytes caps the text of one tools/call result (all content
// items together). Tools that know their data (spool, datasets, USS files)
// already switch to preview + resource_link at the configured
// inline_max_bytes; this cap catches everything else: console and TSO
// output, hardcopy log, list operations, SMP/E queries.
const MaxResultTextBytes = 64 * 1024

// MaxResourceTextBytes caps the text of one resources/read. A resource is
// read on purpose, so the cap is larger than for tool results, but far below
// the 10 MB fetch limit: 10 MB of text in the context cannot work.
const MaxResourceTextBytes = 256 * 1024

// MaxResourceBlobBytes caps base64 blobs (load modules, zFS aggregates).
// Binary content is of no use to a language model; above this size the
// resource is answered with a short text note instead.
const MaxResourceBlobBytes = 16 * 1024

// SanitizeText makes text safe to embed in a JSON-RPC message that a client
// will hold in a model conversation:
//   - invalid UTF-8 becomes U+FFFD,
//   - NEL (U+0085, what EBCDIC X'15' converts to) becomes a line feed,
//   - other C0/C1 control characters (except TAB, LF, CR), DEL, the Unicode
//     line/paragraph separators U+2028/U+2029, the BOM U+FEFF and the
//     noncharacters U+FFFE/U+FFFF become '.', like in a hex dump.
//
// Text without such characters is returned unchanged (no allocation).
func SanitizeText(s string) string {
	if isClean(s) {
		return s
	}
	s = strings.ToValidUTF8(s, string(utf8.RuneError))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == 0x85:
			b.WriteByte('\n')
		case needsReplacement(r):
			b.WriteByte('.')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isClean(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r == 0x85 || needsReplacement(r) {
			return false
		}
	}
	return true
}

func needsReplacement(r rune) bool {
	switch {
	case r == '\t' || r == '\n' || r == '\r':
		return false
	case r < 0x20, r >= 0x7F && r <= 0x9F:
		return true
	case r == 0x2028 || r == 0x2029 || r == 0xFEFF || r == 0xFFFE || r == 0xFFFF:
		return true
	}
	return false
}

// cutText shortens s to at most max bytes, at a line boundary when one lies
// in the second half of the allowed range, and never inside a UTF-8 sequence.
func cutText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 0 {
		return ""
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	head := s[:cut]
	if i := strings.LastIndexByte(head, '\n'); i >= max/2 {
		head = head[:i+1]
	}
	return head
}

func truncationNote(shown, total int) string {
	return fmt.Sprintf("\n[zosmf-mcp: output truncated, %d of %d bytes shown. "+
		"Narrow the request (search/research, record_range, a more specific "+
		"filter or pattern, a shorter time window) instead of repeating it.]\n", shown, total)
}

// GuardToolResult sanitizes all text of a tool result and enforces
// MaxResultTextBytes over all content items together. It reports whether
// anything was truncated.
func GuardToolResult(result *ToolCallResult, max int) bool {
	if result == nil {
		return false
	}
	total := 0
	for i := range result.Content {
		c := &result.Content[i]
		c.Text = SanitizeText(c.Text)
		c.Description = SanitizeText(c.Description)
		total += len(c.Text) + len(c.Data)
	}
	if total <= max {
		return false
	}

	budget := max
	shown := 0
	kept := result.Content[:0]
	for _, c := range result.Content {
		size := len(c.Text) + len(c.Data)
		switch {
		case size <= budget:
			kept = append(kept, c)
			budget -= size
			shown += size
		case c.Type == "text" && budget > 0:
			c.Text = cutText(c.Text, budget)
			kept = append(kept, c)
			shown += len(c.Text)
			budget = 0
		case c.Type == "resource_link":
			// Links are tiny pointers; keep them so the data stays reachable.
			kept = append(kept, c)
		}
	}
	kept = append(kept, TextContent(truncationNote(shown, total)))
	result.Content = kept
	return true
}

// GuardResourceContents sanitizes and caps one resources/read result. It
// reports whether the content was truncated or replaced.
func GuardResourceContents(c *ResourceContents) bool {
	if c.Blob != "" {
		if len(c.Blob) <= MaxResourceBlobBytes {
			return false
		}
		c.Text = fmt.Sprintf("[zosmf-mcp: %s is binary (about %d bytes); not returned because "+
			"binary content cannot be used in a conversation. Read it as text with a "+
			"suitable encoding, or process it on z/OS.]", c.URI, len(c.Blob)*3/4)
		c.Blob = ""
		c.MIMEType = "text/plain"
		return true
	}
	c.Text = SanitizeText(c.Text)
	if len(c.Text) <= MaxResourceTextBytes {
		return false
	}
	total := len(c.Text)
	c.Text = cutText(c.Text, MaxResourceTextBytes)
	c.Text += truncationNote(len(c.Text), total)
	return true
}
