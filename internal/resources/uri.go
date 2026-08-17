// Package resources implements MCP resources for z/OS objects. Datasets,
// USS files and job spool files get addressable URIs so a client can attach
// them as context without a tool call, and so tools can return a pointer
// instead of a payload when the content is large.
package resources

import (
	"fmt"
	"strconv"
	"strings"
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

// DatasetURI builds the URI for a dataset or PDS member.
func DatasetURI(system, dsn string) string {
	return fmt.Sprintf("%s%s/dataset/%s", scheme, strings.ToUpper(system), strings.ToUpper(dsn))
}

// USSURI builds the URI for a USS file. path must be absolute.
func USSURI(system, path string) string {
	return fmt.Sprintf("%s%s/uss%s", scheme, strings.ToUpper(system), path)
}

// SpoolURI builds the URI for one spool file of a job.
func SpoolURI(system, jobName, jobID string, fileID int) string {
	return fmt.Sprintf("%s%s/job/%s/%s/spool/%d",
		scheme, strings.ToUpper(system), strings.ToUpper(jobName), strings.ToUpper(jobID), fileID)
}

// ParseURI turns a zosmf:// URI into a Ref. Dataset and job identifiers are
// upper-cased, since z/OS treats them case-insensitively; USS paths are left
// alone, where case is significant.
func ParseURI(uri string) (Ref, error) {
	if uri == SystemsURI {
		return Ref{Kind: KindSystems}, nil
	}
	if !strings.HasPrefix(uri, scheme) {
		return Ref{}, fmt.Errorf("not a zosmf resource URI: %q", uri)
	}

	rest := strings.TrimPrefix(uri, scheme)
	system, rest, found := strings.Cut(rest, "/")
	if !found || system == "" {
		return Ref{}, fmt.Errorf("missing system in URI: %q", uri)
	}
	system = strings.ToUpper(system)

	switch {
	case strings.HasPrefix(rest, "dataset/"):
		dsn := strings.TrimPrefix(rest, "dataset/")
		if dsn == "" {
			return Ref{}, fmt.Errorf("missing dataset name in URI: %q", uri)
		}
		return Ref{Kind: KindDataset, System: system, Dataset: strings.ToUpper(dsn)}, nil

	case strings.HasPrefix(rest, "uss/"):
		path := strings.TrimPrefix(rest, "uss")
		if path == "" || path == "/" {
			return Ref{}, fmt.Errorf("missing USS path in URI: %q", uri)
		}
		return Ref{Kind: KindUSS, System: system, Path: path}, nil

	case strings.HasPrefix(rest, "job/"):
		parts := strings.Split(strings.TrimPrefix(rest, "job/"), "/")
		if len(parts) != 4 || parts[2] != "spool" {
			return Ref{}, fmt.Errorf("malformed job spool URI: %q", uri)
		}
		id, err := strconv.Atoi(parts[3])
		if err != nil {
			return Ref{}, fmt.Errorf("spool file id is not numeric in URI: %q", uri)
		}
		return Ref{
			Kind:    KindSpool,
			System:  system,
			JobName: strings.ToUpper(parts[0]),
			JobID:   strings.ToUpper(parts[1]),
			FileID:  id,
		}, nil
	}

	return Ref{}, fmt.Errorf("unknown resource kind in URI: %q", uri)
}
