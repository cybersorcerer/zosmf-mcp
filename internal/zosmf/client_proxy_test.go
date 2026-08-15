package zosmf

import (
	"net/http"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
)

func proxyForClient(t *testing.T, c *Client) *http.Request {
	t.Helper()
	tr, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T, want *http.Transport", c.httpClient.Transport)
	}
	if tr.Proxy == nil {
		return nil
	}
	req, err := http.NewRequest(http.MethodGet, "https://zos1.example.com/zosmf/info", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy() returned error: %v", err)
	}
	if u == nil {
		return nil
	}
	return &http.Request{URL: u}
}

func TestNewClientSetsProxyWhenActive(t *testing.T) {
	c, err := NewClient(config.SystemConfig{
		Name:  "SYS1",
		Host:  "https://zos1.example.com",
		Auth:  config.AuthConfig{Type: "basic", Username: "IBMUSER", Password: "secret"},
		Proxy: &config.ProxyConfig{Enabled: true, URL: "socks5://tunnel:1080"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	got := proxyForClient(t, c)
	if got == nil {
		t.Fatal("expected proxy to be configured")
	}
	if got.URL.String() != "socks5://tunnel:1080" {
		t.Errorf("proxy = %q, want socks5://tunnel:1080", got.URL.String())
	}
}

func TestNewClientNoProxyWhenDisabled(t *testing.T) {
	c, err := NewClient(config.SystemConfig{
		Name:  "SYS1",
		Host:  "https://zos1.example.com",
		Auth:  config.AuthConfig{Type: "basic", Username: "IBMUSER", Password: "secret"},
		Proxy: &config.ProxyConfig{Enabled: false, URL: "socks5://tunnel:1080"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := proxyForClient(t, c); got != nil {
		t.Errorf("expected no proxy, got %v", got.URL)
	}
}

func TestNewClientNoProxyWhenAbsent(t *testing.T) {
	c, err := NewClient(config.SystemConfig{
		Name: "SYS1",
		Host: "https://zos1.example.com",
		Auth: config.AuthConfig{Type: "basic", Username: "IBMUSER", Password: "secret"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := proxyForClient(t, c); got != nil {
		t.Errorf("expected no proxy, got %v", got.URL)
	}
}

func TestNewClientRejectsBadProxyURL(t *testing.T) {
	_, err := NewClient(config.SystemConfig{
		Name:  "SYS1",
		Host:  "https://zos1.example.com",
		Auth:  config.AuthConfig{Type: "basic", Username: "IBMUSER", Password: "secret"},
		Proxy: &config.ProxyConfig{Enabled: true, URL: "://nonsense"},
	})
	if err == nil {
		t.Fatal("expected error for malformed proxy URL")
	}
}
