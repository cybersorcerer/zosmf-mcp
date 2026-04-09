package zosmf

import (
	"encoding/json"
	"fmt"
	"strings"
)

// tsoCommandResponse is the response from executing a TSO command.
type tsoCommandResponse struct {
	CmdResponse []struct {
		Message string `json:"message"`
	} `json:"cmdResponse"`
}

// RunTSOCommand executes a TSO/E command via the z/OSMF v1 TSO API.
// The session lifecycle is managed by z/OSMF — no explicit start/stop required.
func (c *Client) RunTSOCommand(cmd, account, proc, charset, codepage string) (string, error) {
	payload := map[string]any{
		"tsoCmd": cmd,
	}

	body, err := c.Put("/zosmf/tsoApp/v1/tso", payload)
	if err != nil {
		return "", fmt.Errorf("executing TSO command: %w", err)
	}

	var resp tsoCommandResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parsing TSO response: %w", err)
	}

	var lines []string
	for _, r := range resp.CmdResponse {
		if r.Message != "" {
			lines = append(lines, r.Message)
		}
	}

	return strings.Join(lines, "\n"), nil
}
