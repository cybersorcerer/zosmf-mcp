package zosmf

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Dataset represents a z/OS dataset as returned by the z/OSMF Datasets REST API.
type Dataset struct {
	DSName string `json:"dsname"`
	DSOrg  string `json:"dsorg,omitempty"`
	VolSer string `json:"vol,omitempty"`
	RecFmt string `json:"recfm,omitempty"`
	LRecL  int    `json:"lrecl,omitempty"`
	BlkSz  int    `json:"blksz,omitempty"`
	CDate  string `json:"cdate,omitempty"`
	RDate  string `json:"rdate,omitempty"`
	EDate  string `json:"edate,omitempty"`
	Used   int    `json:"used,omitempty"`
}

// DatasetMember represents a member of a PDS/PDSE.
type DatasetMember struct {
	Member string `json:"member"`
}

// ListDatasets lists datasets matching a given level pattern (e.g. "SYS1.PARM*").
func (c *Client) ListDatasets(dslevel string) ([]Dataset, error) {
	path := fmt.Sprintf("/zosmf/restfiles/ds?dslevel=%s", url.QueryEscape(dslevel))

	body, err := c.Get(path)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Items []Dataset `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing dataset list response: %w", err)
	}

	return resp.Items, nil
}

// ListMembers lists members of a PDS/PDSE dataset.
func (c *Client) ListMembers(dataset string) ([]DatasetMember, error) {
	path := fmt.Sprintf("/zosmf/restfiles/ds/%s/member", url.PathEscape(dataset))

	body, err := c.Get(path)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Items []DatasetMember `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing member list response: %w", err)
	}

	return resp.Items, nil
}

// ReadDataset reads the content of a sequential dataset or PDS member.
// For a PDS member, use "DATASET.NAME(MEMBER)" format.
func (c *Client) ReadDataset(dsname string) (string, error) {
	path := fmt.Sprintf("/zosmf/restfiles/ds/%s", encodeDatasetPath(dsname))

	reqURL := c.baseURL + path
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-IBM-Data-Type", "text")

	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("read dataset failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("z/OSMF %s returned %d: %s", path, resp.StatusCode, string(body))
	}

	return string(body), nil
}

// WriteDataset writes content to a sequential dataset or PDS member.
// For a PDS member, use "DATASET.NAME(MEMBER)" format.
func (c *Client) WriteDataset(dsname, content string) error {
	path := fmt.Sprintf("/zosmf/restfiles/ds/%s", encodeDatasetPath(dsname))

	reqURL := c.baseURL + path
	req, err := http.NewRequest(http.MethodPut, reqURL, strings.NewReader(content))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-IBM-Data-Type", "text")

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("write dataset failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("z/OSMF %s returned %d: %s", path, resp.StatusCode, string(body))
	}

	return nil
}

// CreateDatasetRequest contains parameters for creating a new dataset.
type CreateDatasetRequest struct {
	DSOrg     string `json:"dsorg"`               // "PS" (sequential) or "PO" (partitioned)
	AlcUnit   string `json:"alcunit"`             // "TRK", "CYL"
	Primary   int    `json:"primary"`             // Primary space
	Secondary int    `json:"secondary"`           // Secondary space
	RecFm     string `json:"recfm"`               // "FB", "VB", "F", "V", "U"
	LRecL     int    `json:"lrecl"`               // Logical record length
	BlkSize   int    `json:"blksize,omitempty"`   // Block size
	DirBlk    int    `json:"dirblk,omitempty"`    // Directory blocks (PO only)
	VolSer    string `json:"volser,omitempty"`    // Volume serial
	Unit      string `json:"unit,omitempty"`      // Unit type
	StorClass string `json:"storclass,omitempty"` // DFSMS storage class
	MgmtClass string `json:"mgntclass,omitempty"` // DFSMS management class
	DataClass string `json:"dataclass,omitempty"` // DFSMS data class
	DSNType   string `json:"dsntype,omitempty"`   // e.g. "EXTREQ" for PDSE
}

// CreateDataset allocates a new z/OS dataset.
func (c *Client) CreateDataset(dsname string, req CreateDatasetRequest) error {
	path := fmt.Sprintf("/zosmf/restfiles/ds/%s", url.PathEscape(dsname))

	body, err := c.Post(path, req)
	if err != nil {
		return fmt.Errorf("create dataset failed: %w", err)
	}
	_ = body
	return nil
}

// DeleteDataset deletes a sequential dataset, PDS, or a PDS member.
// For a member, use "DATASET.NAME(MEMBER)" format.
func (c *Client) DeleteDataset(dsname string) error {
	path := fmt.Sprintf("/zosmf/restfiles/ds/%s", encodeDatasetPath(dsname))
	_, err := c.Delete(path)
	if err != nil {
		return fmt.Errorf("delete dataset failed: %w", err)
	}
	return nil
}

// encodeDatasetPath encodes a dataset name for use in URL paths.
// Handles PDS member notation: "DATASET.NAME(MEMBER)" -> "DATASET.NAME(MEMBER)"
// The parentheses must NOT be percent-encoded for z/OSMF.
func encodeDatasetPath(dsname string) string {
	// If it contains a member reference, encode the parts separately
	if idx := strings.Index(dsname, "("); idx >= 0 {
		ds := dsname[:idx]
		member := dsname[idx:] // includes "(MEMBER)"
		return url.PathEscape(ds) + member
	}
	return url.PathEscape(dsname)
}

// AcquireDatasetEnq acquires an ENQ lock on a dataset or PDS member without
// returning content. enqType must be "exclusive" (SHRW) or "shared" (SHRE).
// Returns the session reference string needed for subsequent operations and release.
func (c *Client) AcquireDatasetEnq(dsname, enqType string) (string, error) {
	var enqHeader string
	switch enqType {
	case "exclusive":
		enqHeader = "SHRW"
	case "shared":
		enqHeader = "SHRE"
	default:
		return "", fmt.Errorf("invalid enq_type: must be 'exclusive' or 'shared'")
	}

	path := fmt.Sprintf("/zosmf/restfiles/ds/%s", encodeDatasetPath(dsname))
	reqURL := c.baseURL + path
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-IBM-Data-Type", "text")
	req.Header.Set("X-IBM-Obtain-ENQ", enqHeader)

	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("acquire ENQ failed: %w", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body) // discard body — content is read separately

	if resp.StatusCode == http.StatusConflict {
		return "", fmt.Errorf("ENQ_NOT_AVAILABLE: %s is locked by another user or session", dsname)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("z/OSMF returned %d acquiring ENQ for %s", resp.StatusCode, dsname)
	}

	sessionRef := resp.Header.Get("X-IBM-Session-Ref")
	return sessionRef, nil
}

// ReleaseDatasetEnq releases a previously acquired ENQ lock identified by sessionRef.
// Errors are non-fatal — z/OS will time out the ENQ automatically if release fails.
func (c *Client) ReleaseDatasetEnq(dsname, sessionRef string) error {
	path := fmt.Sprintf("/zosmf/restfiles/ds/%s", encodeDatasetPath(dsname))
	reqURL := c.baseURL + path
	req, err := http.NewRequest(http.MethodDelete, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-IBM-Session-Ref", sessionRef)

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("release ENQ failed: %w", err)
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body) // discard body

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("z/OSMF returned %d releasing ENQ for %s", resp.StatusCode, dsname)
	}
	return nil
}
