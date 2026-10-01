package config

import (
	"fmt"
	"net/url"
	"strings"
)

// CheckHost reports a z/OSMF host the client cannot call: every request is
// built on it as a base URL, so it needs an http or https scheme and a host.
func CheckHost(host string) error {
	u, err := url.Parse(host)
	if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
		return nil
	}
	bare := host
	if _, rest, found := strings.Cut(host, "://"); found {
		bare = rest
	}
	if bare == "" {
		bare = "zos.example.com"
	}
	return fmt.Errorf("host %q is not a URL — write it with the scheme, e.g. https://%s", host, bare)
}
