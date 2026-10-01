package config

import (
	"strings"
	"testing"
)

func TestCheckHost(t *testing.T) {
	for _, ok := range []string{"https://zos.example.com", "https://192.0.2.10:443", "http://zos:80/"} {
		if err := CheckHost(ok); err != nil {
			t.Errorf("CheckHost(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"192.0.2.10", "192.0.2.10:443", "zos.example.com", "ftp://zos", "https://", ""} {
		err := CheckHost(bad)
		if err == nil {
			t.Errorf("CheckHost(%q) = nil, want an error", bad)
			continue
		}
		if !strings.Contains(err.Error(), "https://") || strings.Contains(err.Error(), "https://ftp") {
			t.Errorf("CheckHost(%q) = %q, want it to show the https:// form", bad, err)
		}
	}
}
