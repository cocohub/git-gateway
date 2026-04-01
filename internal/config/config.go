// Package config handles YAML configuration parsing with environment variable expansion.
package config

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig  `yaml:"server"`
	Log       LogConfig     `yaml:"log"`
	Upstreams []Upstream    `yaml:"upstreams"`
	Agents    []AgentConfig `yaml:"agents"`
}

type ServerConfig struct {
	Listen       string        `yaml:"listen"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type Upstream struct {
	Match      string `yaml:"match"`
	Token      string `yaml:"token"`
	AuthScheme string `yaml:"auth_scheme"`
	Username   string `yaml:"username,omitempty"`
}

type AgentConfig struct {
	ID       string   `yaml:"id"`
	APIKeys  []string `yaml:"api_keys"`
	Policies []Policy `yaml:"policies"`
}

type Policy struct {
	Repos       []string     `yaml:"repos"`
	Allow       []Operation  `yaml:"allow"`
	BranchRules *BranchRules `yaml:"branch_rules,omitempty"`
	PathRules   *PathRules   `yaml:"path_rules,omitempty"`
}

type BranchRules struct {
	AllowPush []string `yaml:"allow_push"`
	DenyPush  []string `yaml:"deny_push"`
}

type PathRules struct {
	DenyModify  []string `yaml:"deny_modify,omitempty"`
	AllowModify []string `yaml:"allow_modify,omitempty"`
}

type Operation string

const (
	OpFetch Operation = "fetch"
	OpClone Operation = "clone"
	OpPush  Operation = "push"
)

var envVarRegex = regexp.MustCompile(`\$\{([^}]+)\}`)

// Load reads and parses the config file, expanding environment variables.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	expanded := expandEnvVars(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	cfg.setDefaults()
	return &cfg, nil
}

func expandEnvVars(s string) string {
	return envVarRegex.ReplaceAllStringFunc(s, func(match string) string {
		varName := envVarRegex.FindStringSubmatch(match)[1]
		return os.Getenv(varName)
	})
}

func (c *Config) setDefaults() {
	if c.Server.Listen == "" {
		c.Server.Listen = ":8080"
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = 30 * time.Second
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = 120 * time.Second
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		c.Log.Format = "json"
	}
	for i := range c.Upstreams {
		if c.Upstreams[i].AuthScheme == "" {
			c.Upstreams[i].AuthScheme = "bearer"
		}
	}
}

func (c *Config) Validate() error {
	if len(c.Upstreams) == 0 {
		return fmt.Errorf("at least one upstream must be configured")
	}
	for i, u := range c.Upstreams {
		if u.Match == "" {
			return fmt.Errorf("upstream[%d]: match pattern is required", i)
		}
		if u.Token == "" {
			return fmt.Errorf("upstream[%d]: token is required", i)
		}
		if u.AuthScheme != "bearer" && u.AuthScheme != "basic" && u.AuthScheme != "" {
			return fmt.Errorf("upstream[%d]: auth_scheme must be 'bearer' or 'basic'", i)
		}
	}
	for i, a := range c.Agents {
		if a.ID == "" {
			return fmt.Errorf("agent[%d]: id is required", i)
		}
		if len(a.APIKeys) == 0 {
			return fmt.Errorf("agent[%d]: at least one api_key is required", i)
		}
		for j, p := range a.Policies {
			if len(p.Repos) == 0 {
				return fmt.Errorf("agent[%d].policy[%d]: at least one repo is required", i, j)
			}
			for _, op := range p.Allow {
				if op != OpFetch && op != OpClone && op != OpPush {
					return fmt.Errorf("agent[%d].policy[%d]: invalid operation %q", i, j, op)
				}
			}
		}
	}
	return nil
}
