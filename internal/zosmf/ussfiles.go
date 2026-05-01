package zosmf

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// USSFile represents a file or directory entry in USS.
type USSFile struct {
	Name   string `json:"name"`
	Mode   string `json:"mode"`
	Size   int64  `json:"size"`
	UID    int    `json:"uid"`
	User   string `json:"user"`
	GID    int    `json:"gid"`
	Group  string `json:"group"`
	MTime  string `json:"mtime"`
	Target string `json:"target,omitempty"` // symlink target
	// Tag is the file tag as returned by z/OSMF with X-IBM-Attributes: get.
	// Values: "t IBM-1047" (EBCDIC), "t ISO8859-1" / "t UTF-8" (ASCII), "b" (binary), "-" (untagged).
	Tag string `json:"tag,omitempty"`
}

// ListUSSFiles lists files in a USS directory, including file tags.
func (c *Client) ListUSSFiles(path string) ([]USSFile, error) {
	apiPath := "/zosmf/restfiles/fs?path=" + url.QueryEscape(path)

	reqURL := c.baseURL + apiPath
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list USS files failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading USS list response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("z/OSMF %s returned %d: %s", apiPath, resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Items []USSFile `json:"items"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("parsing USS file list response: %w", err)
	}

	return result.Items, nil
}

// dataTypeForTag returns the X-IBM-Data-Type header value based on the file tag.
// EBCDIC-tagged files use text conversion; ASCII/UTF-8-tagged and binary files use binary.
// Untagged files default to text (z/OSMF default behaviour).
func dataTypeForTag(tag string) string {
	if tag == "b" {
		return "binary"
	}
	if strings.HasPrefix(tag, "t ") {
		cp := strings.TrimPrefix(tag, "t ")
		// Known EBCDIC code pages — use text conversion, optionally with fileEncoding
		if strings.HasPrefix(cp, "IBM-") || cp == "IBM-1047" {
			if cp != "IBM-1047" {
				return "text;fileEncoding=" + cp
			}
			return "text"
		}
		// ASCII/UTF-8 tagged — fetch as binary, content is already text
		return "binary"
	}
	return "text"
}

// ReadUSSFile reads the content of a USS file.
// tag is the file tag from ListUSSFiles (e.g. "t IBM-1047", "t UTF-8", "b", "-").
// Pass an empty string to use the z/OSMF default (text conversion).
func (c *Client) ReadUSSFile(path, tag string) (string, error) {
	apiPath := "/zosmf/restfiles/fs" + path

	reqURL := c.baseURL + apiPath
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-IBM-Data-Type", dataTypeForTag(tag))

	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("read USS file failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("z/OSMF %s returned %d: %s", apiPath, resp.StatusCode, string(bodyBytes))
	}

	return string(bodyBytes), nil
}

// WriteUSSFile writes content to a USS file.
// tag is the file tag from ListUSSFiles — used to set the correct X-IBM-Data-Type header.
func (c *Client) WriteUSSFile(path, content, tag string) error {
	apiPath := "/zosmf/restfiles/fs" + path

	reqURL := c.baseURL + apiPath
	req, err := http.NewRequest(http.MethodPut, reqURL, strings.NewReader(content))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-IBM-Data-Type", dataTypeForTag(tag))

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("write USS file failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("z/OSMF %s returned %d: %s", apiPath, resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// CreateUSSFile creates a new USS file or directory.
// fileType: "file" or "dir"
// mode: permissions string e.g. "rwxr-xr-x" (optional, defaults to "rwxr-xr-x" for dir, "rw-r--r--" for file)
func (c *Client) CreateUSSFile(path, fileType, mode string) error {
	apiPath := "/zosmf/restfiles/fs" + path

	if mode == "" {
		if fileType == "dir" {
			mode = "rwxr-xr-x"
		} else {
			mode = "rw-r--r--"
		}
	}

	reqBody := map[string]string{
		"type": fileType,
		"mode": mode,
	}

	body, err := c.Post(apiPath, reqBody)
	if err != nil {
		return fmt.Errorf("create USS file failed: %w", err)
	}
	_ = body
	return nil
}

// DeleteUSSFile deletes a USS file or directory.
// recursive: if true, delete non-empty directories recursively
func (c *Client) DeleteUSSFile(path string, recursive bool) error {
	apiPath := "/zosmf/restfiles/fs" + path

	reqURL := c.baseURL + apiPath
	req, err := http.NewRequest(http.MethodDelete, reqURL, nil)
	if err != nil {
		return err
	}

	if recursive {
		req.Header.Set("X-IBM-Option", "recursive")
	}

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("delete USS file failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("z/OSMF %s returned %d: %s", apiPath, resp.StatusCode, string(bodyBytes))
	}

	return nil
}
