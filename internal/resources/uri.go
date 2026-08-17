// Package resources implements MCP resources for z/OS objects. Datasets,
// USS files and job spool files get addressable URIs so a client can attach
// them as context without a tool call, and so tools can return a pointer
// instead of a payload when the content is large.
package resources

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/cybersorcerer/zosmf-mcp/internal/mcp"
)

// Kind is the type of object a URI refers to.
type Kind int

const (
	KindSystems Kind = iota
	KindDataset
	KindUSS
	KindSpool
)

// SystemsURI addresses the server-level system inventory. It uses an empty
// authority so it cannot collide with a system actually named SYSTEMS.
const SystemsURI = "zosmf:///systems"

const scheme = "zosmf://"

// ErrMalformedURI marks a URI the parser cannot make sense of, as opposed to
// a well-formed URI naming an object that does not exist. The server maps it
// to JSON-RPC -32602.
var ErrMalformedURI = fmt.Errorf("%w", mcp.ErrInvalidResourceURI)

// Ref is a parsed resource URI. Which fields are set depends on Kind.
type Ref struct {
	Kind    Kind
	System  string
	Dataset string
	Path    string
	JobName string
	JobID   string
	FileID  int
}

// DatasetURI builds the URI for a dataset or PDS member. Parentheses are
// legal in a URI path (RFC 3986 sub-delims), so SYS1.PARMLIB(IEASYS00) keeps
// its native spelling; the national character '#', which is legal in a
// dataset name but starts a fragment in a URI, is percent-encoded.
func DatasetURI(system, dsn string) string {
	return fmt.Sprintf("%s%s/dataset/%s", scheme, strings.ToUpper(system), escapePath(strings.ToUpper(dsn)))
}

// USSURI builds the URI for a USS file. path must be absolute. The path is
// percent-encoded: a USS file name may contain a space, a '#' or a '%', and a
// spec-conforming client would read everything after a raw '#' as a fragment
// and resolve the wrong file.
func USSURI(system, path string) string {
	return fmt.Sprintf("%s%s/uss%s", scheme, strings.ToUpper(system), escapePath(path))
}

// SpoolURI builds the URI for one spool file of a job.
func SpoolURI(system, jobName, jobID string, fileID int) string {
	return fmt.Sprintf("%s%s/job/%s/%s/spool/%d",
		scheme, strings.ToUpper(system), escapePath(strings.ToUpper(jobName)),
		escapePath(strings.ToUpper(jobID)), fileID)
}

// safeInPath reports whether c may appear literally in a URI path segment.
// It is RFC 3986 pchar minus the percent sign, which only ever introduces an
// escape: unreserved plus sub-delims plus ':' and '@'.
func safeInPath(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		return true
	}
	return strings.IndexByte("-._~!$&'()*+,;=:@", c) >= 0
}

// escapePath percent-encodes a slash-separated path, leaving the separators
// and every character that is legal in a path segment alone. Characters that
// are legal in z/OS names but not in a URI path — notably '#', ' ' and '%' —
// come out encoded.
func escapePath(path string) string {
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		c := path[i]
		if c == '/' || safeInPath(c) {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

// unescapePath reverses escapePath, leaving the separators intact.
func unescapePath(path string) (string, error) {
	segments := strings.Split(path, "/")
	for i, s := range segments {
		decoded, err := url.PathUnescape(s)
		if err != nil {
			return "", err
		}
		segments[i] = decoded
	}
	return strings.Join(segments, "/"), nil
}

// ParseURI turns a zosmf:// URI into a Ref. Dataset and job identifiers are
// upper-cased, since z/OS treats them case-insensitively; USS paths are left
// alone, where case is significant. Every failure wraps ErrMalformedURI.
func ParseURI(uri string) (Ref, error) {
	if uri == SystemsURI {
		return Ref{Kind: KindSystems}, nil
	}
	if !strings.HasPrefix(uri, scheme) {
		return Ref{}, fmt.Errorf("%w: not a zosmf resource URI: %q", ErrMalformedURI, uri)
	}

	rest := strings.TrimPrefix(uri, scheme)
	system, rest, found := strings.Cut(rest, "/")
	if !found || system == "" {
		return Ref{}, fmt.Errorf("%w: missing system in URI: %q", ErrMalformedURI, uri)
	}
	system = strings.ToUpper(system)

	switch {
	case strings.HasPrefix(rest, "dataset/"):
		dsn, err := unescapePath(strings.TrimPrefix(rest, "dataset/"))
		if err != nil {
			return Ref{}, fmt.Errorf("%w: bad escape in dataset name: %q", ErrMalformedURI, uri)
		}
		if dsn == "" {
			return Ref{}, fmt.Errorf("%w: missing dataset name in URI: %q", ErrMalformedURI, uri)
		}
		return Ref{Kind: KindDataset, System: system, Dataset: strings.ToUpper(dsn)}, nil

	case strings.HasPrefix(rest, "uss/"):
		path, err := unescapePath(strings.TrimPrefix(rest, "uss"))
		if err != nil {
			return Ref{}, fmt.Errorf("%w: bad escape in USS path: %q", ErrMalformedURI, uri)
		}
		if path == "" || path == "/" {
			return Ref{}, fmt.Errorf("%w: missing USS path in URI: %q", ErrMalformedURI, uri)
		}
		return Ref{Kind: KindUSS, System: system, Path: path}, nil

	case strings.HasPrefix(rest, "job/"):
		parts := strings.Split(strings.TrimPrefix(rest, "job/"), "/")
		if len(parts) != 4 || parts[2] != "spool" {
			return Ref{}, fmt.Errorf("%w: malformed job spool URI: %q", ErrMalformedURI, uri)
		}
		id, err := strconv.Atoi(parts[3])
		if err != nil {
			return Ref{}, fmt.Errorf("%w: spool file id is not numeric in URI: %q", ErrMalformedURI, uri)
		}
		jobName, err := unescapePath(parts[0])
		if err != nil {
			return Ref{}, fmt.Errorf("%w: bad escape in job name: %q", ErrMalformedURI, uri)
		}
		jobID, err := unescapePath(parts[1])
		if err != nil {
			return Ref{}, fmt.Errorf("%w: bad escape in job id: %q", ErrMalformedURI, uri)
		}
		return Ref{
			Kind:    KindSpool,
			System:  system,
			JobName: strings.ToUpper(jobName),
			JobID:   strings.ToUpper(jobID),
			FileID:  id,
		}, nil
	}

	return Ref{}, fmt.Errorf("%w: unknown resource kind in URI: %q", ErrMalformedURI, uri)
}
