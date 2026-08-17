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

func TestDescribeZOSMFErrorNeverEchoesTheUnderlyingError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "not found",
			err:  errors.New("z/OSMF /zosmf/restfiles/ds/USER.DATA returned 404: {\"details\":\"not cataloged\"}"),
			want: "dataset USER.DATA not found",
		},
		{
			name: "forbidden",
			err:  errors.New("z/OSMF /zosmf/restfiles/ds/USER.DATA returned 403: {\"details\":\"ICH408I\"}"),
			want: "access to dataset USER.DATA denied by SAF/RACF",
		},
		{
			name: "unauthorized with marker",
			err:  errors.New("AUTH_FAILED(S0W1): z/OSMF /zosmf/restfiles/ds/USER.DATA returned 401: bad password"),
			want: "authentication failed for dataset USER.DATA",
		},
		{
			name: "connection failure carries the endpoint but no status",
			err:  errors.New("Get \"https://zosmf.example.com:443/zosmf/restfiles\": dial tcp 192.0.2.3:443: connect: connection refused"),
			want: "reading dataset USER.DATA failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := describeZOSMFError(tt.err, "dataset USER.DATA")
			if got.Error() != tt.want {
				t.Errorf("got %q, want %q", got.Error(), tt.want)
			}
		})
	}
}

func TestDescribeZOSMFErrorDoesNotMatchStatusDigitsInNames(t *testing.T) {
	// A dataset whose name contains 404 must not be reported as "not found"
	// just because of its name.
	err := errors.New("z/OSMF /zosmf/restfiles/ds/USER.T404.DATA returned 500: internal error")

	got := describeZOSMFError(err, "dataset USER.T404.DATA")

	if got.Error() != "reading dataset USER.T404.DATA failed" {
		t.Errorf("got %q, want the generic message", got.Error())
	}
}
