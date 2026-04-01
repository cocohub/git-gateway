package policy

import (
	"fmt"

	"github.com/cocohub/git-gateway/internal/config"
)

// Engine evaluates access policies.
type Engine struct {
	agents map[string]*config.AgentConfig
}

// NewEngine creates a policy engine from agent configs.
func NewEngine(agents []config.AgentConfig) *Engine {
	e := &Engine{
		agents: make(map[string]*config.AgentConfig),
	}
	for i := range agents {
		e.agents[agents[i].ID] = &agents[i]
	}
	return e
}

// CheckOperation evaluates whether agent can perform op on repo.
func (e *Engine) CheckOperation(agentID, repo string, op config.Operation) Decision {
	agent, ok := e.agents[agentID]
	if !ok {
		return Deny(agentID, repo, op, "unknown agent")
	}

	for _, policy := range agent.Policies {
		if !matchesRepo(repo, policy.Repos) {
			continue
		}
		if containsOp(policy.Allow, op) {
			return Allow(agentID, repo, op, "operation allowed by policy")
		}
	}

	return Deny(agentID, repo, op, fmt.Sprintf("operation %s not allowed on repo %s", op, repo))
}

// CheckBranches evaluates whether agent can push to the given refs.
func (e *Engine) CheckBranches(agentID, repo string, updates []RefUpdate) Decision {
	agent, ok := e.agents[agentID]
	if !ok {
		return Deny(agentID, repo, config.OpPush, "unknown agent")
	}

	// Find the matching policy
	var matchedPolicy *config.Policy
	for i := range agent.Policies {
		if matchesRepo(repo, agent.Policies[i].Repos) {
			matchedPolicy = &agent.Policies[i]
			break
		}
	}

	if matchedPolicy == nil {
		return Deny(agentID, repo, config.OpPush, "no policy matches repo")
	}

	// If no branch rules, allow all refs
	if matchedPolicy.BranchRules == nil {
		return Allow(agentID, repo, config.OpPush, "no branch restrictions")
	}

	rules := matchedPolicy.BranchRules

	for _, update := range updates {
		// Deny rules take precedence
		if len(rules.DenyPush) > 0 && MatchAny(update.RefName, rules.DenyPush) {
			return Deny(agentID, repo, config.OpPush,
				fmt.Sprintf("push to %s denied by branch rules", update.RefName))
		}

		// If allow rules exist, ref must match at least one
		if len(rules.AllowPush) > 0 && !MatchAny(update.RefName, rules.AllowPush) {
			return Deny(agentID, repo, config.OpPush,
				fmt.Sprintf("push to %s not in allowed branches", update.RefName))
		}
	}

	return Allow(agentID, repo, config.OpPush, "branch rules satisfied")
}

// CheckPaths evaluates whether agent can modify the given file paths.
func (e *Engine) CheckPaths(agentID, repo string, paths []string) Decision {
	agent, ok := e.agents[agentID]
	if !ok {
		return Deny(agentID, repo, config.OpPush, "unknown agent")
	}

	// Find the matching policy
	var matchedPolicy *config.Policy
	for i := range agent.Policies {
		if matchesRepo(repo, agent.Policies[i].Repos) {
			matchedPolicy = &agent.Policies[i]
			break
		}
	}

	if matchedPolicy == nil {
		return Deny(agentID, repo, config.OpPush, "no policy matches repo")
	}

	// If no path rules, allow all paths
	if matchedPolicy.PathRules == nil {
		return Allow(agentID, repo, config.OpPush, "no path restrictions")
	}

	rules := matchedPolicy.PathRules

	for _, path := range paths {
		// Deny rules take precedence
		if len(rules.DenyModify) > 0 && MatchAny(path, rules.DenyModify) {
			return Deny(agentID, repo, config.OpPush,
				fmt.Sprintf("modification of %s denied by path rules", path))
		}

		// If allow rules exist (allowlist mode), path must match
		if len(rules.AllowModify) > 0 && !MatchAny(path, rules.AllowModify) {
			return Deny(agentID, repo, config.OpPush,
				fmt.Sprintf("modification of %s not in allowed paths", path))
		}
	}

	return Allow(agentID, repo, config.OpPush, "path rules satisfied")
}

// GetPolicy returns the matching policy for an agent and repo, or nil if none.
func (e *Engine) GetPolicy(agentID, repo string) *config.Policy {
	agent, ok := e.agents[agentID]
	if !ok {
		return nil
	}
	for i := range agent.Policies {
		if matchesRepo(repo, agent.Policies[i].Repos) {
			return &agent.Policies[i]
		}
	}
	return nil
}

func matchesRepo(repo string, patterns []string) bool {
	return MatchAny(repo, patterns)
}

func containsOp(ops []config.Operation, op config.Operation) bool {
	for _, o := range ops {
		if o == op {
			return true
		}
	}
	return false
}
