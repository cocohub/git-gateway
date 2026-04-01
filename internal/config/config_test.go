package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Set up test env vars
	os.Setenv("TEST_TOKEN", "secret-token")
	os.Setenv("TEST_KEY", "agent-api-key")
	defer os.Unsetenv("TEST_TOKEN")
	defer os.Unsetenv("TEST_KEY")

	yaml := `
server:
  listen: ":9090"
  read_timeout: 60s
  write_timeout: 180s

log:
  level: "debug"
  format: "text"

upstreams:
  - match: "github.com/**"
    token: "${TEST_TOKEN}"
    auth_scheme: "bearer"

agents:
  - id: "test-agent"
    api_keys:
      - "${TEST_KEY}"
    policies:
      - repos:
          - "github.com/test/repo"
        allow:
          - fetch
          - clone
          - push
        branch_rules:
          allow_push:
            - "refs/heads/feature/**"
          deny_push:
            - "refs/heads/main"
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check server config
	if cfg.Server.Listen != ":9090" {
		t.Errorf("Server.Listen = %q, want %q", cfg.Server.Listen, ":9090")
	}
	if cfg.Server.ReadTimeout != 60*time.Second {
		t.Errorf("Server.ReadTimeout = %v, want %v", cfg.Server.ReadTimeout, 60*time.Second)
	}

	// Check env var expansion
	if cfg.Upstreams[0].Token != "secret-token" {
		t.Errorf("Upstream token = %q, want %q", cfg.Upstreams[0].Token, "secret-token")
	}
	if cfg.Agents[0].APIKeys[0] != "agent-api-key" {
		t.Errorf("Agent API key = %q, want %q", cfg.Agents[0].APIKeys[0], "agent-api-key")
	}

	// Check branch rules
	if len(cfg.Agents[0].Policies[0].BranchRules.AllowPush) != 1 {
		t.Errorf("AllowPush count = %d, want 1", len(cfg.Agents[0].Policies[0].BranchRules.AllowPush))
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				Upstreams: []Upstream{{Match: "github.com/**", Token: "token"}},
				Agents: []AgentConfig{{
					ID:      "agent",
					APIKeys: []string{"key"},
					Policies: []Policy{{
						Repos: []string{"github.com/test/repo"},
						Allow: []Operation{OpFetch},
					}},
				}},
			},
			wantErr: false,
		},
		{
			name: "missing upstream",
			cfg: Config{
				Upstreams: []Upstream{},
				Agents:    []AgentConfig{},
			},
			wantErr: true,
		},
		{
			name: "missing upstream token",
			cfg: Config{
				Upstreams: []Upstream{{Match: "github.com/**"}},
			},
			wantErr: true,
		},
		{
			name: "missing agent ID",
			cfg: Config{
				Upstreams: []Upstream{{Match: "github.com/**", Token: "token"}},
				Agents:    []AgentConfig{{APIKeys: []string{"key"}}},
			},
			wantErr: true,
		},
		{
			name: "invalid operation",
			cfg: Config{
				Upstreams: []Upstream{{Match: "github.com/**", Token: "token"}},
				Agents: []AgentConfig{{
					ID:      "agent",
					APIKeys: []string{"key"},
					Policies: []Policy{{
						Repos: []string{"github.com/test/repo"},
						Allow: []Operation{"invalid"},
					}},
				}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExpandEnvVars(t *testing.T) {
	os.Setenv("TEST_VAR", "expanded")
	defer os.Unsetenv("TEST_VAR")

	input := "token: ${TEST_VAR}"
	result := expandEnvVars(input)

	if result != "token: expanded" {
		t.Errorf("expandEnvVars() = %q, want %q", result, "token: expanded")
	}
}
