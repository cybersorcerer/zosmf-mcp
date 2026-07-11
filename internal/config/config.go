package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Systems    []SystemConfig    `yaml:"systems"`
	Servers    []SystemConfig    `yaml:"servers"`
	SSHSystems []SSHSystemConfig `yaml:"ssh_systems"`
	Transport  TransportConfig   `yaml:"transport"`
}

type SystemConfig struct {
	Name          string     `yaml:"name"`
	Host          string     `yaml:"host"`
	Port          int        `yaml:"port"`
	Auth          AuthConfig `yaml:"auth"`
	CAFile        string     `yaml:"ca_file"`
	SkipTLSVerify bool       `yaml:"skip_tls_verify"`
	CSIDatasets   []string   `yaml:"csi_datasets"`
	TargetZones   []string   `yaml:"target_zones"`
	DlibZones     []string   `yaml:"dlib_zones"`
	TSOAccount    string     `yaml:"tso_account"`
	TSOProc       string     `yaml:"tso_proc"`
	TSOCharset    string     `yaml:"tso_charset"`
	TSOCodepage   string     `yaml:"tso_codepage"`

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
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
}

type SSHSystemConfig struct {
	Name        string            `yaml:"name"`
	Host        string            `yaml:"host"`
	Port        int               `yaml:"port"`
	Username    string            `yaml:"username"`
	KeyFile     string            `yaml:"key_file"`
	Environment map[string]string `yaml:"environment"`
}

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	expanded := expandEnvVars(string(data))

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

func expandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1]
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		return match
	})
}

func (c *Config) normalize() {
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
	}

	for i := range c.SSHSystems {
		if c.SSHSystems[i].Port == 0 {
			c.SSHSystems[i].Port = 22
		}
	}
}

func (c *Config) validate() error {
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
		default:
			return fmt.Errorf("system %s: auth type must be 'basic' or 'certificate', got '%s'", sys.Name, sys.Auth.Type)
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
	candidates := []struct {
		path       string
		deprecated bool
	}{
		{filepath.Join(cwd, DefaultConfigName), false},
		{filepath.Join(home, ".config", "smpe_ls", DefaultConfigName), false},
		{filepath.Join(cwd, LegacyConfigName), true},
	}
	for _, c := range candidates {
		if _, statErr := os.Stat(c.path); statErr == nil {
			return c.path, c.deprecated, nil
		}
	}
	return "", false, fmt.Errorf("no configuration file found; searched: %s, %s, %s",
		candidates[0].path, candidates[1].path, candidates[2].path)
}
