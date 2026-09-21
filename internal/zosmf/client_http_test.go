package zosmf

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
)

// testClient points a real Client at a local test server. Host carries the
// full base URL, which is what NewClient stores.
func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := NewClient(config.SystemConfig{
		Name: "SYS1",
		Host: srv.URL,
		Auth: config.AuthConfig{Type: "basic", Username: "ibmuser", Password: "secret"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// z/OSMF rejects any request that lacks the CSRF header, so every request the
// client builds has to carry it — not just the ones that go through Get.
func TestClientSendsZOSMFHeadersAndBasicAuth(t *testing.T) {
	var gotCSRF, gotAccept, gotUser, gotPass string
	var authOK bool
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotCSRF = r.Header.Get("X-CSRF-ZOSMF-HEADER")
		gotAccept = r.Header.Get("Accept")
		gotUser, gotPass, authOK = r.BasicAuth()
		_, _ = w.Write([]byte(`{}`))
	})

	if _, err := c.Get("/zosmf/restfiles/ds"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotCSRF != "*" {
		t.Errorf("X-CSRF-ZOSMF-HEADER = %q, want *", gotCSRF)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q", gotAccept)
	}
	if !authOK || gotUser != "ibmuser" || gotPass != "secret" {
		t.Errorf("basic auth = %q/%q (ok=%v)", gotUser, gotPass, authOK)
	}
}

// A 401 is marked so the tool layer can tell wrong credentials apart from any
// other failure and prompt for a password instead of retrying.
func TestClientMarksUnauthorized(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("no"))
	})

	_, err := c.Get("/zosmf/restfiles/ds")
	if err == nil {
		t.Fatal("expected an error for 401")
	}
	if !strings.Contains(err.Error(), "AUTH_FAILED(SYS1)") {
		t.Errorf("error = %q, want the AUTH_FAILED marker", err)
	}
}

func TestClientReportsOtherStatusWithBody(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"data set not found"}`))
	})

	_, err := c.Get("/zosmf/restfiles/ds/NOPE")
	if err == nil {
		t.Fatal("expected an error for 404")
	}
	for _, want := range []string{"returned 404", "data set not found"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

// Without the cap an accidental read of a huge dataset materialises in full
// before any higher layer can reject it.
func TestClientRefusesOversizedBody(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		chunk := strings.Repeat("x", 1024*1024)
		for i := 0; i < 11; i++ {
			_, _ = w.Write([]byte(chunk))
		}
	})

	_, err := c.Get("/zosmf/restfiles/ds/BIG")
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("error = %v, want ErrResponseTooLarge", err)
	}
}

func TestClientAcceptsBodyAtTheLimit(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxResponseBytes)))
	})

	body, err := c.Get("/zosmf/restfiles/ds/EXACT")
	if err != nil {
		t.Fatalf("a body of exactly the limit must pass: %v", err)
	}
	if len(body) != maxResponseBytes {
		t.Errorf("len(body) = %d, want %d", len(body), maxResponseBytes)
	}
}

// A system configured without credentials must fail before the request goes
// out, naming the system — otherwise it surfaces as an unexplained 401.
func TestClientWithoutPasswordFailsBeforeSending(t *testing.T) {
	var reached bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(config.SystemConfig{
		Name: "SYS1",
		Host: srv.URL,
		Auth: config.AuthConfig{Type: "basic", Username: "ibmuser"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = c.Get("/zosmf/info")
	if err == nil {
		t.Fatal("expected an error when no password is set")
	}
	if !strings.Contains(err.Error(), "NO_CREDENTIALS(SYS1)") {
		t.Errorf("error = %q, want the NO_CREDENTIALS marker", err)
	}
	if reached {
		t.Error("the request must not reach the server")
	}
}

func TestClientWithoutUserFailsBeforeSending(t *testing.T) {
	c, err := NewClient(config.SystemConfig{Name: "LEGACY", Host: "https://zos.example.com"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = c.Get("/zosmf/info")
	if err == nil || !strings.Contains(err.Error(), "NO_CREDENTIALS(LEGACY)") {
		t.Errorf("error = %v, want NO_CREDENTIALS(LEGACY)", err)
	}
}

// SetPassword exists so the zosmf_set_password tool can supply a password at
// runtime for a system whose configuration carries none.
func TestSetPasswordAppliesToLaterRequests(t *testing.T) {
	var gotPass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotPass, _ = r.BasicAuth()
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(config.SystemConfig{
		Name: "SYS1", Host: srv.URL,
		Auth: config.AuthConfig{Type: "basic", Username: "ibmuser"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	c.SetPassword("late-secret")
	if _, err := c.Get("/zosmf/info"); err != nil {
		t.Fatalf("Get after SetPassword: %v", err)
	}
	if gotPass != "late-secret" {
		t.Errorf("password sent = %q, want late-secret", gotPass)
	}
}

// z/OSMF hands out an LTPA cookie on the first request and expects it back on
// the following ones; without the jar every call pays a fresh logon.
func TestClientKeepsSessionCookies(t *testing.T) {
	var second string
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			http.SetCookie(w, &http.Cookie{Name: "LtpaToken2", Value: "abc", Path: "/"})
		} else {
			second = r.Header.Get("Cookie")
		}
		_, _ = w.Write([]byte(`{}`))
	})

	if _, err := c.Get("/zosmf/info"); err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if _, err := c.Get("/zosmf/info"); err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if !strings.Contains(second, "LtpaToken2=abc") {
		t.Errorf("second request sent Cookie %q, want the LTPA token", second)
	}
}

func TestPostWithStatusReportsStatusAndBody(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"pending"}`))
	})

	status, body, err := c.PostWithStatus("/zosmf/swmgmt/csi", map[string]string{"zone": "GLOBAL"})
	if err != nil {
		t.Fatalf("PostWithStatus: %v", err)
	}
	if status != http.StatusAccepted {
		t.Errorf("status = %d, want 202", status)
	}
	if !strings.Contains(string(body), "pending") {
		t.Errorf("body = %q", body)
	}
}
