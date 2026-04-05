package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Systems   []SystemConfig  `yaml:"systems"`
	Transport TransportConfig `yaml:"transport"`
}

type SystemConfig struct {
	Name          string     `yaml:"name"`
	Host          string     `yaml:"host"`
	Auth          AuthConfig `yaml:"auth"`
	CAFile        string     `yaml:"ca_file"`
	SkipTLSVerify bool       `yaml:"skip_tls_verify"`
	CSIDatasets   []string   `yaml:"csi_datasets"`
	TargetZones   []string   `yaml:"target_zones"`
	DlibZones     []string   `yaml:"dlib_zones"`
}

type AuthConfig struct {
	Type     string `yaml:"type"` // "basic" or "certificate"
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type TransportConfig struct {
	Stdio bool             `yaml:"stdio"`
	HTTP  HTTPConfig       `yaml:"http"`
}

type HTTPConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
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
			if sys.Auth.Username == "" || sys.Auth.Password == "" {
				return fmt.Errorf("system %s: basic auth requires username and password", sys.Name)
			}
		case "certificate":
			if sys.Auth.CertFile == "" || sys.Auth.KeyFile == "" {
				return fmt.Errorf("system %s: certificate auth requires cert_file and key_file", sys.Name)
			}
		default:
			return fmt.Errorf("system %s: auth type must be 'basic' or 'certificate', got '%s'", sys.Name, sys.Auth.Type)
		}
	}

	if !c.Transport.Stdio && !c.Transport.HTTP.Enabled {
		return fmt.Errorf("at least one transport (stdio or http) must be enabled")
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
