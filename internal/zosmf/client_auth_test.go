package zosmf

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/config"
)

// newBasicAuthTestClient returns a Client with basic auth against the given
// httptest server. Password may be empty to simulate missing credentials.
func newBasicAuthTestClient(server *httptest.Server, password string) *Client {
	return &Client{
		httpClient: server.Client(),
		baseURL:    server.URL,
		systemName: "sysa",
		auth:       config.AuthConfig{Type: "basic", Username: "user1", Password: password},
	}
}

func TestDo_BasicAuthWithoutPassword_ReturnsNoCredentialsMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request must not reach the server without credentials")
	}))
	defer srv.Close()
	c := newBasicAuthTestClient(srv, "")

	_, err := c.Get("/zosmf/info")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "NO_CREDENTIALS(SYSA):") {
		t.Errorf("error %q does not contain NO_CREDENTIALS(SYSA):", err.Error())
	}
}

func TestSetPassword_SubsequentRequestUsesNewPassword(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	c := newBasicAuthTestClient(srv, "")

	c.SetPassword("geheim")

	if _, err := c.Get("/zosmf/info"); err != nil {
		t.Fatalf("Get after SetPassword: %v", err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user1:geheim"))
	if capturedAuth != want {
		t.Errorf("Authorization = %q, want %q", capturedAuth, want)
	}
}

func TestDo_EmptyAuthType_SkipsCredentialCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	c := newTestClient(srv) // auth leer — bestehender Helper aus datasets_test.go

	if _, err := c.Get("/zosmf/info"); err != nil {
		t.Errorf("Get with empty auth: %v", err)
	}
}

func TestGet_401_ReturnsAuthFailedMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := newBasicAuthTestClient(srv, "falsch")

	_, err := c.Get("/zosmf/info")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "AUTH_FAILED(SYSA):") {
		t.Errorf("error %q does not contain AUTH_FAILED(SYSA):", err.Error())
	}
}

func TestGet_500_KeepsGenericErrorFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := newBasicAuthTestClient(srv, "pw")

	_, err := c.Get("/zosmf/info")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), "AUTH_FAILED") {
		t.Errorf("500 must not produce AUTH_FAILED marker: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "returned 500") {
		t.Errorf("error %q does not contain generic status message", err.Error())
	}
}
