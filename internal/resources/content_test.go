package resources

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestBuildContentsText(t *testing.T) {
	got, err := buildContents("zosmf://S0W1/dataset/USER.DATA", "text/plain", "hello\nworld\n")
	if err != nil {
		t.Fatalf("buildContents: %v", err)
	}
	if got.Text != "hello\nworld\n" {
		t.Errorf("Text = %q", got.Text)
	}
	if got.Blob != "" {
		t.Error("Blob set on text content")
	}
	if got.MIMEType != "text/plain" {
		t.Errorf("MIMEType = %q, want text/plain", got.MIMEType)
	}
}

func TestBuildContentsBinaryBecomesBlob(t *testing.T) {
	body := string([]byte{0x00, 0xff, 0xfe, 0x01})

	got, err := buildContents("zosmf://S0W1/dataset/SYS1.LINKLIB(IEFBR14)", "text/plain", body)
	if err != nil {
		t.Fatalf("buildContents: %v", err)
	}
	if got.Text != "" {
		t.Error("Text set on binary content")
	}
	if got.MIMEType != "application/octet-stream" {
		t.Errorf("MIMEType = %q, want application/octet-stream", got.MIMEType)
	}
	decoded, err := base64.StdEncoding.DecodeString(got.Blob)
	if err != nil {
		t.Fatalf("Blob is not valid base64: %v", err)
	}
	if string(decoded) != body {
		t.Error("Blob does not round-trip to the original bytes")
	}
}

func TestBuildContentsRejectsOversize(t *testing.T) {
	body := strings.Repeat("a", MaxReadBytes+1)

	_, err := buildContents("zosmf://S0W1/dataset/BIG.DATA", "text/plain", body)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	if !strings.Contains(err.Error(), "10485761") {
		t.Errorf("error should name the actual size, got: %v", err)
	}
}

func TestDescribeZOSMFError404(t *testing.T) {
	err := errors.New("HTTP 404 not found")
	got := describeZOSMFError(err, "dataset")
	expected := "dataset not found"
	if got.Error() != expected {
		t.Errorf("got %q, want %q", got.Error(), expected)
	}
}

func TestDescribeZOSMFError403(t *testing.T) {
	err := errors.New("HTTP 403 forbidden")
	got := describeZOSMFError(err, "file")
	expected := "access to file denied by SAF/RACF"
	if got.Error() != expected {
		t.Errorf("got %q, want %q", got.Error(), expected)
	}
}

func TestDescribeZOSMFError401(t *testing.T) {
	err := errors.New("HTTP 401 unauthorized")
	got := describeZOSMFError(err, "resource")
	expected := "authentication failed for resource"
	if got.Error() != expected {
		t.Errorf("got %q, want %q", got.Error(), expected)
	}
}

func TestDescribeZOSMFErrorFallback(t *testing.T) {
	err := errors.New("some other error")
	got := describeZOSMFError(err, "item")
	expected := "reading item failed"
	if got.Error() != expected {
		t.Errorf("got %q, want %q", got.Error(), expected)
	}
}

func TestDescribeZOSMFErrorDoesNotLeakCredentials(t *testing.T) {
	// Test that URL/hostname credentials are not leaked in error messages
	err := errors.New("HTTP 404 from https://user:password@zosmf.example.com:8080/api/v1")
	got := describeZOSMFError(err, "dataset")

	errMsg := got.Error()

	// Should NOT contain the URL or password
	if strings.Contains(errMsg, "https://") {
		t.Errorf("error message leaked URL: %s", errMsg)
	}
	if strings.Contains(errMsg, "password") {
		t.Errorf("error message leaked password: %s", errMsg)
	}
	if strings.Contains(errMsg, "example.com") {
		t.Errorf("error message leaked hostname: %s", errMsg)
	}

	// Should contain the safe message
	if !strings.Contains(errMsg, "dataset not found") {
		t.Errorf("error message should contain 'dataset not found', got: %s", errMsg)
	}
}

func TestDescribeZOSMFErrorDoesNotLeakInternalHostnames(t *testing.T) {
	// Test with internal hostname in error
	err := errors.New("connection to zosmf-internal.prod.ibm.local:8080 failed: HTTP 403")
	got := describeZOSMFError(err, "file")

	errMsg := got.Error()

	// Should NOT contain the internal hostname
	if strings.Contains(errMsg, "zosmf-internal.prod.ibm.local") {
		t.Errorf("error message leaked internal hostname: %s", errMsg)
	}
	if strings.Contains(errMsg, ".ibm.local") {
		t.Errorf("error message leaked internal domain: %s", errMsg)
	}

	// Should contain the safe message
	if !strings.Contains(errMsg, "access to file denied by SAF/RACF") {
		t.Errorf("error message should contain 'access to file denied by SAF/RACF', got: %s", errMsg)
	}
}
