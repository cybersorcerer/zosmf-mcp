package zosmf

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ConsoleResponse represents the response from issuing a console command.
type ConsoleResponse struct {
	CmdResponse    string `json:"cmd-response"`
	SolKeyDetected bool   `json:"sol-key-detected"`
	CmdResponseKey string `json:"cmd-response-key"`
	DetectionKey   string `json:"detection-key"`
	Status         string `json:"status"`
	Msg            string `json:"msg"`
}

// HardcopyLogItem represents a single entry in the hardcopy log.
type HardcopyLogItem struct {
	Time      string `json:"time"`
	JobName   string `json:"jobName"`
	System    string `json:"system"`
	MessageID string `json:"messageId"`
	Message   string `json:"message"`
	SubType   string `json:"subType"`
}

// HardcopyLogResponse represents the response from a hardcopy log query.
type HardcopyLogResponse struct {
	Source     string            `json:"source"`
	TotalItems int               `json:"totalitems"`
	Items      []HardcopyLogItem `json:"items"`
}

// IssueCommand issues a z/OS operator command synchronously via the z/OSMF Console API.
// consoleName is the EMCS console name (use "defcn" for the default console).
func (c *Client) IssueCommand(consoleName, cmd, system string) (*ConsoleResponse, error) {
	if consoleName == "" {
		consoleName = "defcn"
	}
	path := fmt.Sprintf("/zosmf/restconsoles/consoles/%s", url.PathEscape(consoleName))

	reqBody := map[string]any{
		"cmd": cmd,
	}
	if system != "" {
		reqBody["system"] = system
	}

	body, err := c.Put(path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("issuing command: %w", err)
	}

	var resp ConsoleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing console response: %w", err)
	}

	return &resp, nil
}

// GetConsoleResponse retrieves a delayed console command response by key.
func (c *Client) GetConsoleResponse(consoleName, responseKey string) (string, error) {
	if consoleName == "" {
		consoleName = "defcn"
	}
	path := fmt.Sprintf("/zosmf/restconsoles/consoles/%s/solmsgs/%s",
		url.PathEscape(consoleName), url.PathEscape(responseKey))

	body, err := c.Get(path)
	if err != nil {
		return "", fmt.Errorf("getting console response: %w", err)
	}

	var resp ConsoleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parsing console response: %w", err)
	}

	return resp.CmdResponse, nil
}

// parseAgo converts a relative duration string (e.g. "30m", "2h", "1h30m", "45s")
// to a UNIX timestamp in milliseconds representing now minus that duration.
var agoPattern = regexp.MustCompile(`^(\d+h)?(\d+m)?(\d+s)?$`)

func parseAgo(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if !agoPattern.MatchString(s) || s == "" {
		return 0, fmt.Errorf("invalid ago format %q — examples: 30s, 10m, 2h, 1h30m", s)
	}
	var dur time.Duration
	re := regexp.MustCompile(`(\d+)([hms])`)
	for _, match := range re.FindAllStringSubmatch(s, -1) {
		n, _ := strconv.Atoi(match[1])
		switch match[2] {
		case "h":
			dur += time.Duration(n) * time.Hour
		case "m":
			dur += time.Duration(n) * time.Minute
		case "s":
			dur += time.Duration(n) * time.Second
		}
	}
	if dur == 0 {
		return 0, fmt.Errorf("ago duration must not be zero")
	}
	return time.Now().Add(-dur).UnixMilli(), nil
}

// GetHardcopyLog retrieves hardcopy log messages (OPERLOG or SYSLOG).
//
// source:    "operlog" or "syslog"
// ago:       relative duration string e.g. "10m", "1h", "30s" (preferred)
// since:     absolute start time (used only when ago is empty)
// timeRange: window size e.g. "10m", "1h" (default "10m")
// direction: "forward" or "backward" (default "backward")
// sysName:   only for syslog, filter by system name
func (c *Client) GetHardcopyLog(source string, ago string, since time.Time, timeRange, direction, sysName string) (*HardcopyLogResponse, error) {
	var params []string

	// Timestamp: ago takes priority over since
	if ago != "" {
		ts, err := parseAgo(ago)
		if err != nil {
			return nil, err
		}
		params = append(params, "timestamp="+strconv.FormatInt(ts, 10))
	} else if !since.IsZero() {
		params = append(params, "timestamp="+strconv.FormatInt(since.UnixMilli(), 10))
	}

	if timeRange != "" {
		params = append(params, "timeRange="+timeRange)
	}

	if direction != "" {
		params = append(params, "direction="+direction)
	}

	params = append(params, "hardcopy="+source)

	if source == "syslog" && sysName != "" {
		params = append(params, "sysName="+sysName)
	}

	path := "/zosmf/restconsoles/v1/log?" + strings.Join(params, "&")

	body, err := c.Get(path)
	if err != nil {
		return nil, fmt.Errorf("getting hardcopy log: %w", err)
	}

	var resp HardcopyLogResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing hardcopy log response: %w", err)
	}

	return &resp, nil
}
