package zosmf

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"sync"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/config"
)

// Client is an HTTP client for communicating with a z/OSMF instance.
type Client struct {
	httpClient      *http.Client
	baseURL         string
	mu              sync.RWMutex
	auth            config.AuthConfig
	systemName      string
	missingUsername bool
}

// ClientRegistry manages clients for multiple z/OS systems.
type ClientRegistry struct {
	clients map[string]*Client
}

func NewClientRegistry() *ClientRegistry {
	return &ClientRegistry{
		clients: make(map[string]*Client),
	}
}

func (r *ClientRegistry) Register(name string, client *Client) {
	r.clients[strings.ToUpper(name)] = client
}

func (r *ClientRegistry) Get(name string) (*Client, error) {
	c, ok := r.clients[strings.ToUpper(name)]
	if !ok {
		return nil, fmt.Errorf("unknown system: %s", name)
	}
	return c, nil
}

func (r *ClientRegistry) SystemNames() []string {
	names := make([]string, 0, len(r.clients))
	for name := range r.clients {
		names = append(names, name)
	}
	return names
}

// NewClient creates a z/OSMF client from a system configuration.
func NewClient(sys config.SystemConfig) (*Client, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: sys.SkipTLSVerify,
	}

	// Load custom CA if specified
	if sys.CAFile != "" {
		caCert, err := os.ReadFile(sys.CAFile)
		if err != nil {
			return nil, fmt.Errorf("reading CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = pool
	}

	// Load client certificate if auth type is certificate
	if sys.Auth.Type == "certificate" {
		cert, err := tls.LoadX509KeyPair(sys.Auth.CertFile, sys.Auth.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("loading client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	if sys.Proxy != nil && sys.Proxy.Active() {
		proxyURL, err := url.Parse(sys.Proxy.URL)
		if err != nil {
			return nil, fmt.Errorf("parsing proxy URL for system %s: %w", sys.Name, err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	httpClient := &http.Client{
		Jar:       jar,
		Transport: transport,
	}

	baseURL := strings.TrimRight(sys.Host, "/")

	missingUsername := sys.Auth.Type == ""

	return &Client{
		httpClient:      httpClient,
		baseURL:         baseURL,
		auth:            sys.Auth,
		systemName:      sys.Name,
		missingUsername: missingUsername,
	}, nil
}

// SetPassword replaces the password used for basic auth. It is safe to call
// concurrently with in-flight requests. The password is kept in memory only.
func (c *Client) SetPassword(password string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.auth.Password = password
}

// Do executes an HTTP request against the z/OSMF instance.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	// Set common z/OSMF headers
	req.Header.Set("X-CSRF-ZOSMF-HEADER", "*")
	req.Header.Set("Accept", "application/json")

	if req.Body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.missingUsername {
		return nil, fmt.Errorf("NO_CREDENTIALS(%s): no username configured — add 'user:' to the server entry", strings.ToUpper(c.systemName))
	}

	c.mu.RLock()
	auth := c.auth
	c.mu.RUnlock()

	// Set basic auth if configured
	if auth.Type == "basic" {
		if auth.Password == "" {
			return nil, fmt.Errorf("NO_CREDENTIALS(%s): no password set for this system, set one with the zosmf_set_password tool", strings.ToUpper(c.systemName))
		}
		req.SetBasicAuth(auth.Username, auth.Password)
	}

	return c.httpClient.Do(req)
}

// httpError converts a non-2xx z/OSMF response into an error. 401 is marked
// with the AUTH_FAILED marker so clients can react to wrong credentials.
func (c *Client) httpError(path string, status int, body []byte) error {
	if status == http.StatusUnauthorized {
		return fmt.Errorf("AUTH_FAILED(%s): z/OSMF %s returned 401: %s", strings.ToUpper(c.systemName), path, string(body))
	}
	return fmt.Errorf("z/OSMF %s returned %d: %s", path, status, string(body))
}

// Get performs a GET request to the given z/OSMF API path.
func (c *Client) Get(path string) ([]byte, error) {
	url := c.baseURL + path
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, c.httpError(path, resp.StatusCode, body)
	}

	return body, nil
}

// Put performs a PUT request to the given z/OSMF API path with a JSON body.
func (c *Client) Put(path string, body any) ([]byte, error) {
	return c.doWithBody(http.MethodPut, path, body)
}

// Post performs a POST request to the given z/OSMF API path with a JSON body.
func (c *Client) Post(path string, body any) ([]byte, error) {
	return c.doWithBody(http.MethodPost, path, body)
}

// Delete performs a DELETE request to the given z/OSMF API path.
func (c *Client) Delete(path string) ([]byte, error) {
	url := c.baseURL + path
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}

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
		return nil, c.httpError(path, resp.StatusCode, respBody)
	}

	return respBody, nil
}

// PostWithStatus performs a POST and returns the status code along with the body.
func (c *Client) PostWithStatus(path string, body any) (int, []byte, error) {
	return c.doWithBodyAndStatus(http.MethodPost, path, body)
}

// GetWithStatus performs a GET to a full URL and returns status code and body.
// Used for polling async status URLs.
func (c *Client) GetWithStatus(fullURL string) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return 0, nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request to %s failed: %w", fullURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("reading response body: %w", err)
	}

	return resp.StatusCode, respBody, nil
}

func (c *Client) doWithBodyAndStatus(method, path string, body any) (int, []byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return 0, nil, fmt.Errorf("marshaling request body: %w", err)
	}

	url := c.baseURL + path
	req, err := http.NewRequest(method, url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return 0, nil, err
	}

	resp, err := c.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request to %s failed: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || (resp.StatusCode >= 300 && resp.StatusCode != 202) {
		return resp.StatusCode, nil, c.httpError(path, resp.StatusCode, respBody)
	}

	return resp.StatusCode, respBody, nil
}

func (c *Client) doWithBody(method, path string, body any) ([]byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request body: %w", err)
	}

	url := c.baseURL + path
	req, err := http.NewRequest(method, url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, err
	}

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
		return nil, c.httpError(path, resp.StatusCode, respBody)
	}

	return respBody, nil
}
