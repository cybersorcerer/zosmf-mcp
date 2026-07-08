package zosmf

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Dataset represents a z/OS dataset as returned by the z/OSMF Datasets REST API.
// z/OSMF returns numeric fields as strings when attributes=base is requested.
type Dataset struct {
	DSName  string `json:"dsname"`
	DSOrg   string `json:"dsorg,omitempty"`
	VolSer  string `json:"vol,omitempty"`
	RecFmt  string `json:"recfm,omitempty"`
	LRecL   string `json:"lrecl,omitempty"`
	BlkSz   string `json:"blksz,omitempty"`
	CDate   string `json:"cdate,omitempty"`
	RDate   string `json:"rdate,omitempty"`
	EDate   string `json:"edate,omitempty"`
	Used    string `json:"used,omitempty"`
	Migr    string `json:"migr,omitempty"`
	DSNType string `json:"dsntp,omitempty"`
}

// DatasetMember represents a member of a PDS/PDSE.
type DatasetMember struct {
	Member string `json:"member"`
	Vers   int    `json:"vers,omitempty"`
	Mod    int    `json:"mod,omitempty"`
	C4Date string `json:"c4date,omitempty"`
	M4Date string `json:"m4date,omitempty"`
	CNoRC  int    `json:"cnorc,omitempty"`
	INoRC  int    `json:"inorc,omitempty"`
	MNoRC  int    `json:"mnorc,omitempty"`
	MTime  string `json:"mtime,omitempty"`
	MSec   string `json:"msec,omitempty"`
	User   string `json:"user,omitempty"`
	SCLM   string `json:"sclm,omitempty"`
}

// ListDatasets lists datasets matching a given level pattern (e.g. "SYS1.PARM*").
// volser and start are optional; pass empty strings to omit them.
func (c *Client) ListDatasets(dslevel, volser, start string) ([]Dataset, error) {
	q := url.Values{}
	q.Set("dslevel", dslevel)
	if volser != "" {
		q.Set("volser", volser)
	}
	if start != "" {
		q.Set("start", start)
	}
	reqURL := c.baseURL + "/zosmf/restfiles/ds?" + q.Encode()
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-IBM-Attributes", "base")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to list datasets failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading dataset list response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("z/OSMF list datasets returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Items []Dataset `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing dataset list response: %w", err)
	}

	return result.Items, nil
}

// ListMembers lists members of a PDS/PDSE dataset.
func (c *Client) ListMembers(dataset string) ([]DatasetMember, error) {
	reqURL := c.baseURL + fmt.Sprintf("/zosmf/restfiles/ds/%s/member", url.PathEscape(dataset))
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-IBM-Attributes", "base")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to list members failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading member list response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("z/OSMF list members returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Items []DatasetMember `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing member list response: %w", err)
	}

	return result.Items, nil
}

// ReadDatasetResult is the structured result of a ReadDataset/ReadDatasetEx
// call. NumRecords / FirstMatch are populated from the X-IBM-Record-Range
// response header when a search was requested.
type ReadDatasetResult struct {
	Content    string `json:"content"`
	Found      bool   `json:"found"`
	FirstMatch int    `json:"first_match"`
	NumRecords int    `json:"num_records"`
}

// ReadDataset reads the content of a sequential dataset or PDS member.
// For a PDS member, use "DATASET.NAME(MEMBER)" format.
// Kept for callers that don't need search support.
func (c *Client) ReadDataset(dsname, encoding string) (string, error) {
	res, err := c.ReadDatasetEx(dsname, encoding, "", true, 0)
	if err != nil {
		return "", err
	}
	return res.Content, nil
}

// ReadDatasetEx reads the content of a sequential dataset or PDS member, with
// optional server-side search.
//
// search        - substring; z/OSMF returns the content starting at the first
//
//	matching record (max maxReturnSize records).
//
// insensitive   - default true; pass false for case-sensitive match.
// maxReturnSize - max number of records returned (z/OSMF default 100).
//
// When search is empty the function behaves like a plain read.
// When search is set, the response also exposes the X-IBM-Record-Range
// header parsed into FirstMatch (1-based start) and NumRecords. Found is
// true iff NumRecords > 0.
func (c *Client) ReadDatasetEx(dsname, encoding, search string, insensitive bool, maxReturnSize int) (*ReadDatasetResult, error) {
	path := fmt.Sprintf("/zosmf/restfiles/ds/%s", encodeDatasetPath(dsname))

	if search != "" {
		q := url.Values{}
		q.Set("search", search)
		if !insensitive {
			q.Set("insensitive", "false")
		}
		if maxReturnSize > 0 {
			q.Set("maxreturnsize", strconv.Itoa(maxReturnSize))
		}
		path = path + "?" + q.Encode()
	}

	reqURL := c.baseURL + path
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	if encoding != "" {
		req.Header.Set("X-IBM-Data-Type", "text;fileEncoding="+encoding)
	} else {
		req.Header.Set("X-IBM-Data-Type", "text")
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("read dataset failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("z/OSMF %s returned %d: %s", path, resp.StatusCode, string(body))
	}

	res := &ReadDatasetResult{
		Content: string(body),
		Found:   true, // assume found unless we see X-IBM-Record-Range
	}
	if rr := resp.Header.Get("X-IBM-Record-Range"); rr != "" {
		var p, q int
		if _, err := fmt.Sscanf(rr, "%d,%d", &p, &q); err == nil {
			res.FirstMatch = p
			res.NumRecords = q
			res.Found = q > 0
		}
	}
	return res, nil
}

// WriteDataset writes content to a sequential dataset or PDS member.
// For a PDS member, use "DATASET.NAME(MEMBER)" format.
// releaseEnq controls whether X-IBM-Release-ENQ: true is sent — set to false
// when the editor tab remains open and the ENQ should be kept for subsequent saves.
func (c *Client) WriteDataset(dsname, content, sessionRef, encoding string, releaseEnq bool) error {
	path := fmt.Sprintf("/zosmf/restfiles/ds/%s", encodeDatasetPath(dsname))

	reqURL := c.baseURL + path
	req, err := http.NewRequest(http.MethodPut, reqURL, strings.NewReader(content))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")
	if encoding != "" {
		req.Header.Set("X-IBM-Data-Type", "text;fileEncoding="+encoding)
	} else {
		req.Header.Set("X-IBM-Data-Type", "text")
	}
	if sessionRef != "" {
		req.Header.Set("X-IBM-Session-Ref", sessionRef)
		if releaseEnq {
			req.Header.Set("X-IBM-Release-ENQ", "true")
		}
	}

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

// AcquireDatasetEnq acquires an ENQ lock on a dataset or PDS member and returns
// the content along with the session reference. enqType must be "exclusive" (SHRW)
// or "shared" (SHRE).
func (c *Client) AcquireDatasetEnq(dsname, enqType, encoding string) (content, sessionRef string, err error) {
	var enqHeader string
	switch enqType {
	case "exclusive":
		enqHeader = "SHRW"
	case "shared":
		enqHeader = "SHRE"
	default:
		return "", "", fmt.Errorf("invalid enq_type: must be 'exclusive' or 'shared'")
	}

	path := fmt.Sprintf("/zosmf/restfiles/ds/%s", encodeDatasetPath(dsname))
	reqURL := c.baseURL + path
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", "", err
	}
	if encoding != "" {
		req.Header.Set("X-IBM-Data-Type", "text;fileEncoding="+encoding)
	} else {
		req.Header.Set("X-IBM-Data-Type", "text")
	}
	req.Header.Set("X-IBM-Obtain-ENQ", enqHeader)

	resp, err := c.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("acquire ENQ failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("reading ENQ response body: %w", err)
	}

	if resp.StatusCode == http.StatusConflict {
		return "", "", fmt.Errorf("ENQ_NOT_AVAILABLE: %s is locked by another user or session", dsname)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("z/OSMF returned %d acquiring ENQ for %s", resp.StatusCode, dsname)
	}

	sessionRef = resp.Header.Get("X-IBM-Session-Ref")
	if sessionRef == "" {
		return "", "", fmt.Errorf("z/OSMF did not return X-IBM-Session-Ref for %s", dsname)
	}
	return string(body), sessionRef, nil
}

// ReleaseDatasetEnq releases a previously acquired ENQ lock identified by sessionRef.
// Per z/OSMF documentation: send GET /zosmf/restfiles/ping with X-IBM-Session-Ref
// and X-IBM-Release-ENQ: true to release the ENQ held by the TSO address space.
func (c *Client) ReleaseDatasetEnq(dsname, sessionRef string) error {
	reqURL := c.baseURL + "/zosmf/restfiles/ping"
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-IBM-Session-Ref", sessionRef)
	req.Header.Set("X-IBM-Release-ENQ", "true")

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("release ENQ failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("z/OSMF returned %d releasing ENQ for %s", resp.StatusCode, dsname)
	}
	return nil
}
