package zosmf

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TSOAddressSpace selects where a TSO command runs. With an Account, the
// TSO/E address space service starts an address space with exactly these
// settings. Without one, the single-request service runs the command, and
// z/OSMF chooses the settings itself — it takes none from the caller.
type TSOAddressSpace struct {
	Account  string
	Proc     string
	Charset  string
	Codepage string
}

// The settings an address space gets when the configuration leaves them
// out — the same defaults Zowe CLI uses.
const (
	defaultTSOProc     = "IZUFPROC"
	defaultTSOCharset  = "697"
	defaultTSOCodepage = "1047"
	tsoRows            = "24"
	tsoColumns         = "80"
	tsoRegionSize      = "4096"
)

// How often, and how long, to poll an address space for TSO's prompt. The
// tests shorten both.
var (
	tsoPollInterval = 100 * time.Millisecond
	tsoTimeout      = 5 * time.Minute
)

// tsoCommandResponse is the response of the single-request service.
type tsoCommandResponse struct {
	CmdResponse []struct {
		Message string `json:"message"`
	} `json:"cmdResponse"`
}

// tsoServiceResponse is what the address space service returns for every
// request: the address space's key, z/OSMF's own messages, and the TSO data
// that has arrived since the last request.
type tsoServiceResponse struct {
	ServletKey string `json:"servletKey"`
	MsgData    []struct {
		MessageText string `json:"messageText"`
	} `json:"msgData"`
	TSOData []map[string]struct {
		Data string `json:"DATA"`
	} `json:"tsoData"`
}

// RunTSOCommand runs one TSO/E command and returns its output.
func (c *Client) RunTSOCommand(cmd string, as TSOAddressSpace) (string, error) {
	if as.Account == "" {
		return c.runTSOCommandStateless(cmd)
	}
	return c.runTSOCommandInAddressSpace(cmd, as)
}

func (c *Client) runTSOCommandStateless(cmd string) (string, error) {
	body, err := c.Put("/zosmf/tsoApp/v1/tso", map[string]any{"tsoCmd": cmd})
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

// runTSOCommandInAddressSpace starts an address space with the configured
// settings, sends the command, collects its output and stops the address
// space again.
func (c *Client) runTSOCommandInAddressSpace(cmd string, as TSOAddressSpace) (string, error) {
	proc := orDefault(as.Proc, defaultTSOProc)
	query := url.Values{
		"acct":  {as.Account},
		"proc":  {proc},
		"chset": {orDefault(as.Charset, defaultTSOCharset)},
		"cpage": {orDefault(as.Codepage, defaultTSOCodepage)},
		"rows":  {tsoRows},
		"cols":  {tsoColumns},
		"rsize": {tsoRegionSize},
	}
	start, err := c.tsoCall(func() ([]byte, error) {
		return c.requestWithoutBody(http.MethodPost, "/zosmf/tsoApp/tso?"+query.Encode())
	})
	if err != nil {
		return "", fmt.Errorf("starting a TSO address space: %w", err)
	}
	if start.ServletKey == "" {
		return "", fmt.Errorf("starting a TSO address space: z/OSMF returned no servlet key")
	}
	path := "/zosmf/tsoApp/tso/" + url.PathEscape(start.ServletKey)
	// Best effort: z/OSMF also ends an address space that stays idle.
	defer func() { _, _ = c.Delete(path) }()

	logon, err := c.collectTSO(path, start)
	if err != nil {
		return "", fmt.Errorf("starting a TSO address space: %w", err)
	}
	for _, line := range logon {
		if strings.Contains(line, "IKJ56482I") {
			return "", fmt.Errorf("TSO logon with procedure %s failed: %s", proc, strings.Join(logon, "\n"))
		}
	}

	sent, err := c.tsoCall(func() ([]byte, error) {
		return c.Put(path+"?readReply=false", map[string]any{
			"TSO RESPONSE": map[string]string{"VERSION": "0100", "DATA": cmd},
		})
	})
	if err != nil {
		return "", fmt.Errorf("sending the TSO command: %w", err)
	}
	out, err := c.collectTSO(path, sent)
	if err != nil {
		return "", fmt.Errorf("reading the TSO command's output: %w", err)
	}
	return strings.Join(out, "\n"), nil
}

// collectTSO gathers TSO messages, polling the address space, until TSO
// prompts for input again. A prompt that comes before any message ends the
// previous exchange, not this one, and is skipped — as Zowe CLI does.
func (c *Client) collectTSO(path string, resp tsoServiceResponse) ([]string, error) {
	deadline := time.Now().Add(tsoTimeout)
	var lines []string
	for {
		for _, item := range resp.TSOData {
			if msg, ok := item["TSO MESSAGE"]; ok {
				lines = append(lines, msg.Data)
			} else if _, ok := item["TSO PROMPT"]; ok && len(lines) > 0 {
				return lines, nil
			}
		}
		if time.Now().After(deadline) {
			return lines, fmt.Errorf("no TSO prompt within %s", tsoTimeout)
		}
		if len(resp.TSOData) == 0 {
			time.Sleep(tsoPollInterval)
		}
		next, err := c.tsoCall(func() ([]byte, error) { return c.Get(path) })
		if err != nil {
			return lines, err
		}
		resp = next
	}
}

// tsoCall runs one request against the address space service and turns an
// error message z/OSMF puts into the response body into an error.
func (c *Client) tsoCall(request func() ([]byte, error)) (tsoServiceResponse, error) {
	var resp tsoServiceResponse
	body, err := request()
	if err != nil {
		return resp, err
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return resp, fmt.Errorf("parsing TSO response: %w", err)
	}
	if len(resp.MsgData) > 0 {
		return resp, fmt.Errorf("z/OSMF: %s", resp.MsgData[0].MessageText)
	}
	return resp, nil
}

// requestWithoutBody sends a request that carries no body at all — starting
// an address space is a POST with its parameters in the query string only.
func (c *Client) requestWithoutBody(method, path string) ([]byte, error) {
	req, err := http.NewRequest(method, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := readLimited(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, c.httpError(path, resp.StatusCode, body)
	}
	return body, nil
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
