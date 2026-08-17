package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type ProxyConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
}

// Active reports whether requests should be routed through the proxy.
func (p ProxyConfig) Active() bool {
	return p.Enabled && p.URL != ""
}

type SSHProxyJumpConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
}

// Active reports whether SSH connections should be routed via a jump host.
func (s SSHProxyJumpConfig) Active() bool {
	return s.Enabled && s.Host != ""
}

type Config struct {
	Systems      []SystemConfig     `yaml:"systems"`
	Servers      []SystemConfig     `yaml:"servers"`
	SSHSystems   []SSHSystemConfig  `yaml:"ssh_systems"`
	Transport    TransportConfig    `yaml:"transport"`
	Proxy        ProxyConfig        `yaml:"proxy"`
	SSHProxyJump SSHProxyJumpConfig `yaml:"ssh_proxy_jump"`
	Resources    ResourcesConfig    `yaml:"resources"`
}

// ResourcesConfig tunes how much content a tool result carries inline before
// it switches to a resource_link.
type ResourcesConfig struct {
	InlineMaxBytes int `yaml:"inline_max_bytes"`
}

// defaultInlineMaxBytes mirrors tools.DefaultInlineMaxBytes. Duplicated
// rather than imported: internal/config must not depend on internal/tools.
const defaultInlineMaxBytes = 32768

type SystemConfig struct {
	Name          string       `yaml:"name"`
	Host          string       `yaml:"host"`
	Port          int          `yaml:"port"`
	Auth          AuthConfig   `yaml:"auth"`
	CAFile        string       `yaml:"ca_file"`
	SkipTLSVerify bool         `yaml:"skip_tls_verify"`
	CSIDatasets   []string     `yaml:"csi_datasets"`
	TargetZones   []string     `yaml:"target_zones"`
	DlibZones     []string     `yaml:"dlib_zones"`
	TSOAccount    string       `yaml:"tso_account"`
	TSOProc       string       `yaml:"tso_proc"`
	TSOCharset    string       `yaml:"tso_charset"`
	TSOCodepage   string       `yaml:"tso_codepage"`
	Proxy         *ProxyConfig `yaml:"proxy"`

	// smpe_ls-compatible aliases
	User               string   `yaml:"user"`
	RejectUnauthorized *bool    `yaml:"rejectUnauthorized"`
	CSI                any      `yaml:"csi"`
	Zones              []string `yaml:"zones"`
	DefaultZones       []string `yaml:"defaultZones"`
}

type AuthConfig struct {
	Type     string `yaml:"type"` // "basic" or "certificate"
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type TransportConfig struct {
	Stdio          bool       `yaml:"stdio"`
	HTTP           HTTPConfig `yaml:"http"`
	StreamableHTTP HTTPConfig `yaml:"streamable_http"`
}

type HTTPConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Listen        string `yaml:"listen"`
	APIKey        string `yaml:"api_key"`
	AllowInsecure bool   `yaml:"allow_insecure"`
}

type SSHSystemConfig struct {
	Name        string              `yaml:"name"`
	Host        string              `yaml:"host"`
	Port        int                 `yaml:"port"`
	Username    string              `yaml:"username"`
	KeyFile     string              `yaml:"key_file"`
	Environment map[string]string   `yaml:"environment"`
	ProxyJump   *SSHProxyJumpConfig `yaml:"proxy_jump"`
}

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	expanded, missing := expandEnvVars(string(data))
	if len(missing) > 0 {
		return nil, fmt.Errorf("unresolved environment variables in %s: %s — "+
			"set them, or remove the ${...} references from the file",
			path, strings.Join(missing, ", "))
	}

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if len(cfg.Systems) == 0 && len(cfg.Servers) > 0 {
		cfg.Systems = cfg.Servers
	}

	cfg.normalize()

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// expandEnvVars substitutes ${VAR} references and reports the names of any
// that are not set. An unresolved reference used to survive as its own literal
// text, which turned a missing password into an unexplained 401 much later —
// so the caller refuses to start instead.
func expandEnvVars(s string) (string, []string) {
	var missing []string
	seen := make(map[string]bool)

	expanded := envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1]
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		if !seen[varName] {
			seen[varName] = true
			missing = append(missing, varName)
		}
		return match
	})

	return expanded, missing
}

func (c *Config) normalize() {
	if c.Resources.InlineMaxBytes <= 0 {
		c.Resources.InlineMaxBytes = defaultInlineMaxBytes
	}

	// Setting ZOSMF_PROXY is an explicit act, so it enables the proxy on its
	// own — a config file that only wants to supply the URL can still set
	// proxy.url, and a per-system block can opt out again.
	if c.Proxy.URL == "" {
		if envProxy := os.Getenv("ZOSMF_PROXY"); envProxy != "" {
			c.Proxy.URL = envProxy
			c.Proxy.Enabled = true
		}
	}

	// The API key may come from the environment so a mounted production
	// config never has to carry it. An explicit api_key in the file wins.
	if envKey := os.Getenv("MCP_API_KEY"); envKey != "" {
		if c.Transport.HTTP.APIKey == "" {
			c.Transport.HTTP.APIKey = envKey
		}
		if c.Transport.StreamableHTTP.APIKey == "" {
			c.Transport.StreamableHTTP.APIKey = envKey
		}
	}

	envPassword := os.Getenv("ZOSMF_PASSWORD")

	for i := range c.Systems {
		s := &c.Systems[i]

		// user: → auth.username
		if s.User != "" && s.Auth.Username == "" {
			s.Auth.Username = s.User
		}

		// password from env if not in config
		if s.Auth.Password == "" && envPassword != "" {
			s.Auth.Password = envPassword
		}

		// default auth type to basic if username+password are set
		if s.Auth.Type == "" && s.Auth.Username != "" {
			s.Auth.Type = "basic"
		}

		// rejectUnauthorized: false → skip_tls_verify: true
		if s.RejectUnauthorized != nil && !*s.RejectUnauthorized {
			s.SkipTLSVerify = true
		}

		// port: → append to host URL if not already present
		if s.Port != 0 {
			host := s.Host
			if !strings.Contains(host[strings.LastIndex(host, "/")+1:], ":") {
				s.Host = fmt.Sprintf("%s:%d", strings.TrimRight(host, "/"), s.Port)
			}
		}

		// csi: (string or list) → csi_datasets
		if len(s.CSIDatasets) == 0 && s.CSI != nil {
			switch v := s.CSI.(type) {
			case string:
				s.CSIDatasets = []string{v}
			case []any:
				for _, item := range v {
					if str, ok := item.(string); ok {
						s.CSIDatasets = append(s.CSIDatasets, str)
					}
				}
			}
		}

		// zones: → target_zones
		if len(s.TargetZones) == 0 && len(s.Zones) > 0 {
			s.TargetZones = s.Zones
		}

		// proxy: global default, per-system override wins
		if s.Proxy == nil {
			global := c.Proxy
			s.Proxy = &global
		} else if s.Proxy.URL == "" {
			s.Proxy.URL = c.Proxy.URL
		}
	}

	for i := range c.SSHSystems {
		if c.SSHSystems[i].Port == 0 {
			c.SSHSystems[i].Port = 22
		}

		if c.SSHSystems[i].ProxyJump == nil {
			global := c.SSHProxyJump
			c.SSHSystems[i].ProxyJump = &global
		} else if c.SSHSystems[i].ProxyJump.Host == "" {
			c.SSHSystems[i].ProxyJump.Host = c.SSHProxyJump.Host
		}
	}
}

func (c *Config) validate() error {
	if err := validateProxy(c.Proxy); err != nil {
		return err
	}

	if err := validateSSHProxyJump(c.SSHProxyJump); err != nil {
		return err
	}

	if len(c.Systems) == 0 {
		return fmt.Errorf("at least one system must be configured")
	}

	names := make(map[string]bool)
	for i, sys := range c.Systems {
		if sys.Name == "" {
			return fmt.Errorf("system %d: name is required", i)
		}
		normalized := strings.ToUpper(sys.Name)
		if names[normalized] {
			return fmt.Errorf("duplicate system name: %s", sys.Name)
		}
		names[normalized] = true

		if sys.Host == "" {
			return fmt.Errorf("system %s: host is required", sys.Name)
		}

		switch sys.Auth.Type {
		case "basic":
			if sys.Auth.Username == "" {
				return fmt.Errorf("system %s: basic auth requires username", sys.Name)
			}
		case "certificate":
			if sys.Auth.CertFile == "" || sys.Auth.KeyFile == "" {
				return fmt.Errorf("system %s: certificate auth requires cert_file and key_file", sys.Name)
			}
		case "":
			if sys.Auth.CertFile != "" || sys.Auth.KeyFile != "" {
				return fmt.Errorf("system %s: certificate files configured but auth type missing — set \"type: certificate\"", sys.Name)
			}
			// Tolerated: legacy smpe_ls entries without user/auth. The client
			// rejects tool calls with a NO_CREDENTIALS marker instead of
			// blocking server startup for all other systems.
		default:
			return fmt.Errorf("system %s: auth type must be 'basic' or 'certificate', got '%s'", sys.Name, sys.Auth.Type)
		}

		if sys.Proxy != nil {
			if err := validateProxy(*sys.Proxy); err != nil {
				return fmt.Errorf("system %s: %w", sys.Name, err)
			}
		}
	}

	sshNames := make(map[string]bool)
	for i, ssh := range c.SSHSystems {
		if ssh.Name == "" {
			return fmt.Errorf("ssh_systems entry %d: name is required", i)
		}
		normalized := strings.ToUpper(ssh.Name)
		if sshNames[normalized] {
			return fmt.Errorf("duplicate ssh_systems name: %s", ssh.Name)
		}
		sshNames[normalized] = true
		if ssh.Host == "" {
			return fmt.Errorf("ssh_systems %s: host is required", ssh.Name)
		}
		if ssh.Username == "" {
			return fmt.Errorf("ssh_systems %s: username is required", ssh.Name)
		}
		if ssh.ProxyJump != nil {
			if err := validateSSHProxyJump(*ssh.ProxyJump); err != nil {
				return fmt.Errorf("ssh_systems %s: %w", ssh.Name, err)
			}
		}
	}

	return nil
}

func (c *Config) SystemByName(name string) (*SystemConfig, error) {
	normalized := strings.ToUpper(name)
	for i := range c.Systems {
		if strings.ToUpper(c.Systems[i].Name) == normalized {
			return &c.Systems[i], nil
		}
	}
	return nil, fmt.Errorf("system not found: %s", name)
}

func (c *Config) SystemNames() []string {
	names := make([]string, len(c.Systems))
	for i, sys := range c.Systems {
		names[i] = sys.Name
	}
	return names
}

// DefaultConfigName is the canonical config shared by smpe_ls, vscpdf,
// nvpdf and zosmf-mcp. LegacyConfigName is kept for backwards compatibility.
const (
	DefaultConfigName = ".smpe-zosmf.yaml"
	LegacyConfigName  = ".zosmf-mcp-config.yaml"
)

// ResolveDefaultPath locates the configuration file when --config is not
// given: cwd, then ~/.config/smpe_ls/, then the deprecated legacy name in
// cwd. deprecated is true when the legacy file was chosen.
func ResolveDefaultPath(cwd, home string) (path string, deprecated bool, err error) {
	type candidate struct {
		path       string
		deprecated bool
	}
	var candidates []candidate
	candidates = append(candidates, candidate{filepath.Join(cwd, DefaultConfigName), false})
	if home != "" {
		candidates = append(candidates, candidate{filepath.Join(home, ".config", "smpe_ls", DefaultConfigName), false})
	}
	candidates = append(candidates, candidate{filepath.Join(cwd, LegacyConfigName), true})

	var searched []string
	var statErrs []error
	for _, c := range candidates {
		searched = append(searched, c.path)
		if _, statErr := os.Stat(c.path); statErr == nil {
			return c.path, c.deprecated, nil
		} else if !os.IsNotExist(statErr) {
			statErrs = append(statErrs, statErr)
		}
	}
	msg := fmt.Sprintf("no configuration file found; searched: %s", strings.Join(searched, ", "))
	if len(statErrs) > 0 {
		errStrs := make([]string, len(statErrs))
		for i, e := range statErrs {
			errStrs[i] = e.Error()
		}
		msg += fmt.Sprintf("; errors: %s", strings.Join(errStrs, "; "))
	}
	return "", false, fmt.Errorf("%s", msg)
}

// ValidateHTTPAuth checks the api_key requirement for the transports that
// are actually being started. Call this after the --transport flag has been
// resolved, not from Load(), so that a config with http.enabled: true but no
// api_key still loads fine when the process is started with --transport stdio.
func (c *Config) ValidateHTTPAuth(useHTTP, useStreamableHTTP bool) error {
	if useHTTP {
		if err := validateHTTPAuth("http", c.Transport.HTTP); err != nil {
			return err
		}
	}
	if useStreamableHTTP {
		if err := validateHTTPAuth("streamable_http", c.Transport.StreamableHTTP); err != nil {
			return err
		}
	}
	return nil
}

// validateHTTPAuth is only reached for a transport that is actually going to
// run — the caller resolves that from config and the --transport flag. It must
// not consult h.Enabled: a transport forced on from the command line would
// otherwise skip the api_key requirement and listen unauthenticated.
func validateHTTPAuth(name string, h HTTPConfig) error {
	if h.AllowInsecure || h.APIKey != "" {
		return nil
	}
	return fmt.Errorf("transport %s: api_key is required — set it, or set allow_insecure: true to run without authentication", name)
}

func validateProxy(p ProxyConfig) error {
	if p.Enabled && p.URL == "" {
		return fmt.Errorf("proxy is enabled but url is not set")
	}
	if p.URL == "" {
		return nil
	}
	u, err := url.Parse(p.URL)
	if err != nil {
		return fmt.Errorf("parsing proxy URL: invalid URL")
	}
	switch u.Scheme {
	case "http", "https", "socks5":
	default:
		return fmt.Errorf("proxy URL scheme must be http, https or socks5, got '%s'", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("proxy URL is missing a host: %s", u.Redacted())
	}
	return nil
}

func validateSSHProxyJump(s SSHProxyJumpConfig) error {
	if s.Enabled && s.Host == "" {
		return fmt.Errorf("ssh_proxy_jump is enabled but host is not set")
	}
	return nil
}
