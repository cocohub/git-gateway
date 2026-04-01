// Package policy implements access control evaluation.
package policy

import "github.com/cocohub/git-gateway/internal/config"

// Decision represents the result of a policy evaluation.
type Decision struct {
	Allowed bool
	Reason  string
	Agent   string
	Repo    string
	Op      config.Operation
}

// Allow creates an allowed decision.
func Allow(agent, repo string, op config.Operation, reason string) Decision {
	return Decision{
		Allowed: true,
		Reason:  reason,
		Agent:   agent,
		Repo:    repo,
		Op:      op,
	}
}

// Deny creates a denied decision.
func Deny(agent, repo string, op config.Operation, reason string) Decision {
	return Decision{
		Allowed: false,
		Reason:  reason,
		Agent:   agent,
		Repo:    repo,
		Op:      op,
	}
}

// RefUpdate represents a git ref update from a push operation.
type RefUpdate struct {
	OldSHA  string
	NewSHA  string
	RefName string
}

// IsCreate returns true if this is a new ref creation.
func (r RefUpdate) IsCreate() bool {
	return r.OldSHA == "0000000000000000000000000000000000000000"
}

// IsDelete returns true if this is a ref deletion.
func (r RefUpdate) IsDelete() bool {
	return r.NewSHA == "0000000000000000000000000000000000000000"
}
