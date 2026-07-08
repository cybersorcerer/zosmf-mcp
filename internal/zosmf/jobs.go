package zosmf

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Job represents a z/OS job as returned by the z/OSMF Jobs REST API.
type Job struct {
	JobID         string    `json:"jobid"`
	JobName       string    `json:"jobname"`
	Owner         string    `json:"owner"`
	Status        string    `json:"status"`
	Type          string    `json:"type"`
	Class         string    `json:"class"`
	RetCode       string    `json:"retcode"`
	SubSystem     string    `json:"subsystem,omitempty"`
	URL           string    `json:"url,omitempty"`
	Phase         int       `json:"phase,omitempty"`
	PhaseName     string    `json:"phase-name,omitempty"`
	ExecSystem    string    `json:"exec-system,omitempty"`
	ExecMember    string    `json:"exec-member,omitempty"`
	ExecSubmitted string    `json:"exec-submitted,omitempty"`
	ExecStarted   string    `json:"exec-started,omitempty"`
	ExecEnded     string    `json:"exec-ended,omitempty"`
	StepData      []JobStep `json:"step-data,omitempty"`
}

// JobStep represents a single completed step returned when step-data=Y is requested.
type JobStep struct {
	StepNumber     int    `json:"step-number,omitempty"`
	StepName       string `json:"step-name,omitempty"`
	ProcStepName   string `json:"proc-step-name,omitempty"`
	ProgramName    string `json:"program-name,omitempty"`
	CompletionCode string `json:"completion-code,omitempty"`
	ActiveInd      bool   `json:"active-ind,omitempty"`
}

// JobFile represents a spool file (DD) of a job.
type JobFile struct {
	ID         int    `json:"id"`
	DDName     string `json:"ddname"`
	RecordFmt  string `json:"record-format,omitempty"`
	ByteCount  int64  `json:"byte-count,omitempty"`
	RecCount   int    `json:"record-count,omitempty"`
	StepName   string `json:"stepname,omitempty"`
	ProcStep   string `json:"procstep,omitempty"`
	Class      string `json:"class,omitempty"`
	RecordsURL string `json:"records-url,omitempty"`
}

// ListJobsParams contains parameters for listing jobs.
type ListJobsParams struct {
	Owner    string
	Prefix   string
	JobID    string // mutually exclusive with UserCorrelator
	Status   string // ACTIVE, OUTPUT, INPUT, or empty for all
	MaxJobs  int
	ExecData bool // request exec-started/exec-ended/exec-member/exec-system fields
}

// ListJobs retrieves a list of jobs matching the given criteria.
func (c *Client) ListJobs(params ListJobsParams) ([]Job, error) {
	query := url.Values{}
	if params.Owner != "" {
		query.Set("owner", params.Owner)
	}
	if params.Prefix != "" {
		query.Set("prefix", params.Prefix)
	}
	if params.JobID != "" {
		query.Set("jobid", params.JobID)
	}
	if params.Status != "" {
		query.Set("status", params.Status)
	}
	if params.MaxJobs > 0 {
		query.Set("max-jobs", fmt.Sprintf("%d", params.MaxJobs))
	}
	if params.ExecData {
		query.Set("exec-data", "Y")
	}

	path := "/zosmf/restjobs/jobs"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	body, err := c.Get(path)
	if err != nil {
		return nil, err
	}

	var jobs []Job
	if err := json.Unmarshal(body, &jobs); err != nil {
		return nil, fmt.Errorf("parsing jobs response: %w", err)
	}

	return jobs, nil
}

// GetJob retrieves details for a specific job, including exec-data and step-data.
func (c *Client) GetJob(jobName, jobID string) (*Job, error) {
	path := fmt.Sprintf("/zosmf/restjobs/jobs/%s/%s?exec-data=Y&step-data=Y",
		url.PathEscape(jobName), url.PathEscape(jobID))

	body, err := c.Get(path)
	if err != nil {
		return nil, err
	}

	var job Job
	if err := json.Unmarshal(body, &job); err != nil {
		return nil, fmt.Errorf("parsing job response: %w", err)
	}

	return &job, nil
}

// GetJobFiles retrieves the spool files for a job.
func (c *Client) GetJobFiles(jobName, jobID string) ([]JobFile, error) {
	path := fmt.Sprintf("/zosmf/restjobs/jobs/%s/%s/files",
		url.PathEscape(jobName), url.PathEscape(jobID))

	body, err := c.Get(path)
	if err != nil {
		return nil, err
	}

	var files []JobFile
	if err := json.Unmarshal(body, &files); err != nil {
		return nil, fmt.Errorf("parsing job files response: %w", err)
	}

	return files, nil
}

// GetJobFileContent retrieves the content of a specific spool file.
func (c *Client) GetJobFileContent(jobName, jobID string, fileID int) (string, error) {
	path := fmt.Sprintf("/zosmf/restjobs/jobs/%s/%s/files/%d/records",
		url.PathEscape(jobName), url.PathEscape(jobID), fileID)

	body, err := c.Get(path)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// SubmitJobJCL submits a job from inline JCL.
func (c *Client) SubmitJobJCL(jcl string) (*Job, error) {
	path := "/zosmf/restjobs/jobs"
	reqURL := c.baseURL + path

	req, err := http.NewRequest(http.MethodPut, reqURL, strings.NewReader(jcl))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-IBM-Data-Type", "text")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("submit job failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("z/OSMF submit returned %d: %s", resp.StatusCode, string(body))
	}

	var job Job
	if err := json.Unmarshal(body, &job); err != nil {
		return nil, fmt.Errorf("parsing submit response: %w", err)
	}

	return &job, nil
}

// SubmitJobDataset submits a job from a dataset.
func (c *Client) SubmitJobDataset(dataset string) (*Job, error) {
	reqBody := map[string]string{
		"file": fmt.Sprintf("//'%s'", dataset),
	}

	body, err := c.Put("/zosmf/restjobs/jobs", reqBody)
	if err != nil {
		return nil, err
	}

	var job Job
	if err := json.Unmarshal(body, &job); err != nil {
		return nil, fmt.Errorf("parsing submit response: %w", err)
	}

	return &job, nil
}

// CancelJob cancels a running job.
func (c *Client) CancelJob(jobName, jobID string) error {
	path := fmt.Sprintf("/zosmf/restjobs/jobs/%s/%s",
		url.PathEscape(jobName), url.PathEscape(jobID))

	reqBody := map[string]string{
		"request": "cancel",
		"version": "2.0",
	}

	_, err := c.Put(path, reqBody)
	return err
}

// PurgeJob purges (deletes) a job's output.
func (c *Client) PurgeJob(jobName, jobID string) error {
	path := fmt.Sprintf("/zosmf/restjobs/jobs/%s/%s",
		url.PathEscape(jobName), url.PathEscape(jobID))

	_, err := c.Delete(path)
	return err
}

// HoldJob places a job on hold.
func (c *Client) HoldJob(jobName, jobID string) error {
	path := fmt.Sprintf("/zosmf/restjobs/jobs/%s/%s",
		url.PathEscape(jobName), url.PathEscape(jobID))

	reqBody := map[string]string{
		"request": "hold",
		"version": "2.0",
	}

	_, err := c.Put(path, reqBody)
	return err
}

// ReleaseJob releases a held job.
func (c *Client) ReleaseJob(jobName, jobID string) error {
	path := fmt.Sprintf("/zosmf/restjobs/jobs/%s/%s",
		url.PathEscape(jobName), url.PathEscape(jobID))

	reqBody := map[string]string{
		"request": "release",
		"version": "2.0",
	}

	_, err := c.Put(path, reqBody)
	return err
}
