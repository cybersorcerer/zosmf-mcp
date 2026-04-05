package zosmf

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// CSIQueryRequest represents a request to the SMP/E CSI Query API.
type CSIQueryRequest struct {
	CSIDataset string   `json:"-"`                  // used in URL path, not in body
	Zones      []string `json:"zones"`
	Entries    []string `json:"entries"`
	Subentries []string `json:"subentries,omitempty"`
	Filter     string   `json:"filter,omitempty"`
}

// CSIQueryResponse represents the response from the SMP/E CSI Query API.
type CSIQueryResponse struct {
	Entries []CSIEntry `json:"entries,omitempty"`
	Return  *CSIReturn `json:"return,omitempty"`
}

// CSIEntry represents a single CSI entry returned by z/OSMF.
type CSIEntry struct {
	EntryName  string                      `json:"entryname"`
	EntryType  string                      `json:"entrytype"`
	ZoneName   string                      `json:"zonename"`
	Subentries []map[string]json.RawMessage `json:"subentries,omitempty"`
}

// CSIReturn contains return/reason codes from the CSI query.
type CSIReturn struct {
	ReturnCode int    `json:"returnCode"`
	ReasonCode int    `json:"reasonCode"`
	Message    string `json:"message,omitempty"`
}

// asyncResponse is the z/OSMF 202 response containing a status URL to poll.
type asyncResponse struct {
	StatusURL string `json:"statusurl"`
}

// asyncResult wraps the polling result which contains a status field and the actual entries.
type asyncResult struct {
	Status  string     `json:"status"`
	Entries []CSIEntry `json:"entries,omitempty"`
}

// QueryCSI executes an SMP/E CSI query against the z/OSMF SMP/E REST API.
// The API is asynchronous: it returns 202 with a status URL, which must be
// polled until the result is ready (200).
func (c *Client) QueryCSI(req CSIQueryRequest) (*CSIQueryResponse, error) {
	path := fmt.Sprintf("/zosmf/swmgmt/csi/csiquery/%s", url.PathEscape(req.CSIDataset))

	statusCode, body, err := c.PostWithStatus(path, req)
	if err != nil {
		return nil, fmt.Errorf("CSI query failed: %w", err)
	}

	// If we got 202, poll the status URL for the actual result
	if statusCode == 202 {
		var async asyncResponse
		if err := json.Unmarshal(body, &async); err != nil {
			return nil, fmt.Errorf("parsing async response: %w", err)
		}
		if async.StatusURL == "" {
			return nil, fmt.Errorf("202 response missing statusurl")
		}
		body, err = c.pollStatusURL(async.StatusURL)
		if err != nil {
			return nil, fmt.Errorf("polling CSI result: %w", err)
		}

		// Polling result has a wrapper with status + entries
		var result asyncResult
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("parsing async result: %w", err)
		}
		return &CSIQueryResponse{Entries: result.Entries}, nil
	}

	// Direct (non-async) response
	var resp CSIQueryResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing CSI response: %w", err)
	}

	if resp.Return != nil && resp.Return.ReturnCode != 0 {
		return &resp, fmt.Errorf("CSI query returned RC=%d RS=%d: %s",
			resp.Return.ReturnCode, resp.Return.ReasonCode, resp.Return.Message)
	}

	return &resp, nil
}

// pollStatusURL polls a z/OSMF async status URL until the result is ready.
// Returns 200 response body on success. Polls up to 60 seconds.
func (c *Client) pollStatusURL(statusURL string) ([]byte, error) {
	const maxAttempts = 30
	const pollInterval = 2 * time.Second

	for i := 0; i < maxAttempts; i++ {
		time.Sleep(pollInterval)

		statusCode, body, err := c.GetWithStatus(statusURL)
		if err != nil {
			return nil, err
		}

		switch {
		case statusCode == 200:
			return body, nil
		case statusCode == 202:
			// Still processing, continue polling
			continue
		default:
			return nil, fmt.Errorf("unexpected status %d while polling: %s", statusCode, string(body))
		}
	}

	return nil, fmt.Errorf("CSI query timed out after %d seconds", maxAttempts*int(pollInterval.Seconds()))
}
