package zosmf

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serveBytes returns a server that writes n bytes without ever buffering them,
// so the test can offer far more data than the client is allowed to keep.
func serveBytes(t *testing.T, n int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.CopyN(w, zeroReader{}, n); err != nil {
			return // client hung up after the limit was hit; expected
		}
	}))
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

func TestGet_OversizedResponseIsRejected(t *testing.T) {
	srv := serveBytes(t, maxResponseBytes+1024)
	defer srv.Close()
	c := newBasicAuthTestClient(srv, "secret")

	_, err := c.Get("/zosmf/restfiles/ds/HUGE.DATASET")
	if err == nil {
		t.Fatal("expected an error for an oversized response, got nil")
	}
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("error %q is not ErrResponseTooLarge", err.Error())
	}
}

func TestGet_ResponseAtTheLimitIsAccepted(t *testing.T) {
	srv := serveBytes(t, maxResponseBytes)
	defer srv.Close()
	c := newBasicAuthTestClient(srv, "secret")

	body, err := c.Get("/zosmf/restfiles/ds/BIG.DATASET")
	if err != nil {
		t.Fatalf("a body of exactly the limit must be accepted: %v", err)
	}
	if len(body) != maxResponseBytes {
		t.Errorf("got %d bytes, want %d", len(body), maxResponseBytes)
	}
}

func TestReadUSSFile_OversizedResponseIsRejected(t *testing.T) {
	srv := serveBytes(t, maxResponseBytes+1024)
	defer srv.Close()
	c := newBasicAuthTestClient(srv, "secret")

	_, err := c.ReadUSSFile("/u/user1/huge.bin", "")
	if err == nil {
		t.Fatal("expected an error for an oversized response, got nil")
	}
	if !strings.Contains(err.Error(), "10 MB limit") {
		t.Errorf("error %q does not mention the limit", err.Error())
	}
}
