package policy

import (
	"testing"

	"github.com/cocohub/git-gateway/internal/config"
)

func TestCheckOperation(t *testing.T) {
	agents := []config.AgentConfig{
		{
			ID:      "reader",
			APIKeys: []string{"key"},
			Policies: []config.Policy{{
				Repos: []string{"github.com/acme/**"},
				Allow: []config.Operation{config.OpFetch},
			}},
		},
		{
			ID:      "writer",
			APIKeys: []string{"key"},
			Policies: []config.Policy{{
				Repos: []string{"github.com/acme/repo"},
				Allow: []config.Operation{config.OpFetch, config.OpPush},
			}},
		},
	}

	engine := NewEngine(agents)

	tests := []struct {
		name    string
		agent   string
		repo    string
		op      config.Operation
		allowed bool
	}{
		{
			name:    "reader can fetch",
			agent:   "reader",
			repo:    "github.com/acme/repo",
			op:      config.OpFetch,
			allowed: true,
		},
		{
			name:    "reader can fetch any repo with glob",
			agent:   "reader",
			repo:    "github.com/acme/any-repo",
			op:      config.OpFetch,
			allowed: true,
		},
		{
			name:    "reader cannot push",
			agent:   "reader",
			repo:    "github.com/acme/repo",
			op:      config.OpPush,
			allowed: false,
		},
		{
			name:    "writer can push",
			agent:   "writer",
			repo:    "github.com/acme/repo",
			op:      config.OpPush,
			allowed: true,
		},
		{
			name:    "writer cannot access other repo",
			agent:   "writer",
			repo:    "github.com/acme/other",
			op:      config.OpFetch,
			allowed: false,
		},
		{
			name:    "unknown agent denied",
			agent:   "unknown",
			repo:    "github.com/acme/repo",
			op:      config.OpFetch,
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := engine.CheckOperation(tt.agent, tt.repo, tt.op)
			if decision.Allowed != tt.allowed {
				t.Errorf("CheckOperation() = %v, want %v; reason: %s",
					decision.Allowed, tt.allowed, decision.Reason)
			}
		})
	}
}

func TestCheckBranches(t *testing.T) {
	agents := []config.AgentConfig{
		{
			ID:      "agent",
			APIKeys: []string{"key"},
			Policies: []config.Policy{{
				Repos: []string{"github.com/acme/repo"},
				Allow: []config.Operation{config.OpPush},
				BranchRules: &config.BranchRules{
					AllowPush: []string{"refs/heads/feature/**", "refs/heads/agent/**"},
					DenyPush:  []string{"refs/heads/main", "refs/tags/**"},
				},
			}},
		},
		{
			ID:      "unrestricted",
			APIKeys: []string{"key"},
			Policies: []config.Policy{{
				Repos: []string{"github.com/acme/repo"},
				Allow: []config.Operation{config.OpPush},
				// No branch rules = all branches allowed
			}},
		},
	}

	engine := NewEngine(agents)

	tests := []struct {
		name    string
		agent   string
		updates []RefUpdate
		allowed bool
	}{
		{
			name:  "push to feature branch allowed",
			agent: "agent",
			updates: []RefUpdate{{
				OldSHA:  "abc123",
				NewSHA:  "def456",
				RefName: "refs/heads/feature/my-feature",
			}},
			allowed: true,
		},
		{
			name:  "push to main denied",
			agent: "agent",
			updates: []RefUpdate{{
				OldSHA:  "abc123",
				NewSHA:  "def456",
				RefName: "refs/heads/main",
			}},
			allowed: false,
		},
		{
			name:  "push to tag denied",
			agent: "agent",
			updates: []RefUpdate{{
				OldSHA:  "0000000000000000000000000000000000000000",
				NewSHA:  "def456",
				RefName: "refs/tags/v1.0.0",
			}},
			allowed: false,
		},
		{
			name:  "push to random branch not in allow list",
			agent: "agent",
			updates: []RefUpdate{{
				OldSHA:  "abc123",
				NewSHA:  "def456",
				RefName: "refs/heads/random",
			}},
			allowed: false,
		},
		{
			name:  "unrestricted agent can push to main",
			agent: "unrestricted",
			updates: []RefUpdate{{
				OldSHA:  "abc123",
				NewSHA:  "def456",
				RefName: "refs/heads/main",
			}},
			allowed: true,
		},
		{
			name:  "deny takes precedence over allow",
			agent: "agent",
			updates: []RefUpdate{
				{OldSHA: "abc", NewSHA: "def", RefName: "refs/heads/feature/ok"},
				{OldSHA: "abc", NewSHA: "def", RefName: "refs/heads/main"},
			},
			allowed: false, // main is denied even though feature is allowed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := engine.CheckBranches(tt.agent, "github.com/acme/repo", tt.updates)
			if decision.Allowed != tt.allowed {
				t.Errorf("CheckBranches() = %v, want %v; reason: %s",
					decision.Allowed, tt.allowed, decision.Reason)
			}
		})
	}
}

func TestCheckPaths(t *testing.T) {
	agents := []config.AgentConfig{
		{
			ID:      "blocklist-agent",
			APIKeys: []string{"key"},
			Policies: []config.Policy{{
				Repos: []string{"github.com/acme/repo"},
				Allow: []config.Operation{config.OpPush},
				PathRules: &config.PathRules{
					DenyModify: []string{".github/workflows/**", "go.mod", "Dockerfile"},
				},
			}},
		},
		{
			ID:      "allowlist-agent",
			APIKeys: []string{"key"},
			Policies: []config.Policy{{
				Repos: []string{"github.com/acme/repo"},
				Allow: []config.Operation{config.OpPush},
				PathRules: &config.PathRules{
					AllowModify: []string{"src/**", "tests/**"},
				},
			}},
		},
	}

	engine := NewEngine(agents)

	tests := []struct {
		name    string
		agent   string
		paths   []string
		allowed bool
	}{
		{
			name:    "blocklist: normal file allowed",
			agent:   "blocklist-agent",
			paths:   []string{"src/main.go", "src/handler.go"},
			allowed: true,
		},
		{
			name:    "blocklist: workflow denied",
			agent:   "blocklist-agent",
			paths:   []string{"src/main.go", ".github/workflows/ci.yml"},
			allowed: false,
		},
		{
			name:    "blocklist: go.mod denied",
			agent:   "blocklist-agent",
			paths:   []string{"go.mod"},
			allowed: false,
		},
		{
			name:    "allowlist: src allowed",
			agent:   "allowlist-agent",
			paths:   []string{"src/main.go", "src/lib/util.go"},
			allowed: true,
		},
		{
			name:    "allowlist: docs not allowed",
			agent:   "allowlist-agent",
			paths:   []string{"docs/readme.md"},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := engine.CheckPaths(tt.agent, "github.com/acme/repo", tt.paths)
			if decision.Allowed != tt.allowed {
				t.Errorf("CheckPaths() = %v, want %v; reason: %s",
					decision.Allowed, tt.allowed, decision.Reason)
			}
		})
	}
}
