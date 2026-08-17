package resources

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
)

// MaxReadBytes caps a single resources/read. The client asked for this
// object deliberately, so there is no threshold — but an accidental read of a
// very large dataset must not take the process down. Not configurable.
const MaxReadBytes = 10 * 1024 * 1024

// ErrTooLarge is returned when a resource body exceeds MaxReadBytes.
var ErrTooLarge = errors.New("resource too large")

// buildContents packages a fetched body. Content that is not valid UTF-8 —
// load modules, zFS aggregates — goes out as a base64 blob rather than as
// broken text.
func buildContents(uri, mimeType, body string) (mcp.ResourceContents, error) {
	if len(body) > MaxReadBytes {
		return mcp.ResourceContents{}, fmt.Errorf("%w: %d bytes exceeds the %d byte limit",
			ErrTooLarge, len(body), MaxReadBytes)
	}

	if !utf8.ValidString(body) {
		return mcp.ResourceContents{
			URI:      uri,
			MIMEType: "application/octet-stream",
			Blob:     base64.StdEncoding.EncodeToString([]byte(body)),
		}, nil
	}

	return mcp.ResourceContents{URI: uri, MIMEType: mimeType, Text: body}, nil
}

// describeZOSMFError reduces a z/OSMF failure to its statement. Response
// headers and request URLs are never passed through — they can carry
// credentials or internal hostnames.
func describeZOSMFError(err error, subject string) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "404"):
		return fmt.Errorf("%s not found", subject)
	case strings.Contains(msg, "403"):
		return fmt.Errorf("access to %s denied by SAF/RACF", subject)
	case strings.Contains(msg, "401"):
		return fmt.Errorf("authentication failed for %s", subject)
	}
	return fmt.Errorf("reading %s failed", subject)
}
