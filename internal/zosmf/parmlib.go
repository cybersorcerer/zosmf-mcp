package zosmf

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ParmlibValidateResult is the response from the z/OSMF parmlib validate API.
type ParmlibValidateResult struct {
	Result                string           `json:"result"`
	NumberOfMembers       int              `json:"numberOfMembers"`
	NumberOfFailedMembers int              `json:"numberOfFailedMembers"`
	ParmlibDatasets       []ParmlibDataset `json:"parmlibDatasets"`
	Details               json.RawMessage  `json:"details"`
}

// ParmlibDataset represents a dataset in the parmlib concatenation.
type ParmlibDataset struct {
	Volser  string `json:"volser"`
	Dataset string `json:"dataset"`
}

// ParmlibValidateOptions controls the validate request.
type ParmlibValidateOptions struct {
	// MemberType is the parmlib type without xx suffix (e.g. "BPXPRM", "IEASYS").
	MemberType string
	// Member is an optional specific member name (e.g. "BPXPRM00").
	Member string
	// Dataset is the dataset containing the member.
	Dataset string
	// Volser is optional if the dataset is not cataloged.
	Volser string
	// LoadMember is an optional LOADxx member name for type resolution.
	LoadMember string
	// LoadDataset is the dataset containing the LOADxx member.
	LoadDataset string
	// LoadVolser is optional if LoadDataset is not cataloged.
	LoadVolser string
	// Deep enables deep validation (only valid for LOAD type).
	Deep bool
	// System is an optional remote system name in the sysplex.
	System string
	// Content is optional inline parmlib content to validate (PUT body).
	Content string
}

// ValidateParmlib validates parmlib member syntax using the z/OSMF REST API.
func (c *Client) ValidateParmlib(opts ParmlibValidateOptions) (*ParmlibValidateResult, error) {
	memberType := strings.ToUpper(opts.MemberType)
	path := fmt.Sprintf("/zosmf/parmlib/v1/%s/validate", memberType)

	var params []string
	if opts.System != "" {
		params = append(params, "system="+opts.System)
	}
	if opts.Deep {
		params = append(params, "deep=true")
	}
	if opts.Member != "" {
		params = append(params, "member="+opts.Member)
		if opts.Dataset != "" {
			params = append(params, "dataset="+opts.Dataset)
		}
		if opts.Volser != "" {
			params = append(params, "volser="+opts.Volser)
		}
	} else if opts.Content == "" {
		if opts.LoadMember != "" {
			params = append(params, "memLOAD="+opts.LoadMember)
			if opts.LoadDataset != "" {
				params = append(params, "dsnLOAD="+opts.LoadDataset)
			}
			if opts.LoadVolser != "" {
				params = append(params, "volsLOAD="+opts.LoadVolser)
			}
		}
	}

	if len(params) > 0 {
		path += "?" + strings.Join(params, "&")
	}

	var (
		body []byte
		err  error
	)
	if opts.Content != "" {
		body, err = c.putRaw(path, []byte(opts.Content), "text/plain")
	} else {
		body, err = c.Put(path, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("parmlib validate: %w", err)
	}

	var result ParmlibValidateResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing parmlib validate response: %w", err)
	}
	return &result, nil
}

// putRaw performs a PUT with a raw byte body and explicit content type.
func (c *Client) putRaw(path string, body []byte, contentType string) ([]byte, error) {
	url := c.baseURL + path
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("z/OSMF %s returned %d: %s", path, resp.StatusCode, string(respBody))
	}
	return respBody, nil
}
