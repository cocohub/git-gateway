// Package auth handles agent authentication via API keys.
package auth

import (
	"errors"
	"net/http"

	"github.com/cocohub/git-gateway/internal/config"
)

var (
	ErrNoCredentials = errors.New("no credentials provided")
	ErrInvalidAPIKey = errors.New("invalid API key")
	ErrUnknownAgent  = errors.New("unknown agent")
)

// Authenticator resolves an incoming request to an agent identity.
type Authenticator interface {
	Authenticate(r *http.Request) (agentID string, err error)
}

// APIKeyAuthenticator checks Basic Auth or X-Gateway-Token header.
type APIKeyAuthenticator struct {
	agentsByKey map[string]string // API key -> agent ID
	agentIDs    map[string]bool   // valid agent IDs
}

// NewAPIKeyAuthenticator creates an authenticator from agent configs.
func NewAPIKeyAuthenticator(agents []config.AgentConfig) *APIKeyAuthenticator {
	auth := &APIKeyAuthenticator{
		agentsByKey: make(map[string]string),
		agentIDs:    make(map[string]bool),
	}
	for _, agent := range agents {
		auth.agentIDs[agent.ID] = true
		for _, key := range agent.APIKeys {
			if key == "" {
				continue // Skip empty keys (env var not set)
			}
			auth.agentsByKey[key] = agent.ID
		}
	}
	return auth
}

// Authenticate extracts agent identity from the request.
// Supports Basic Auth (username=agent-id, password=api-key) and X-Gateway-Token header.
func (a *APIKeyAuthenticator) Authenticate(r *http.Request) (string, error) {
	// Try X-Gateway-Token header first
	if token := r.Header.Get("X-Gateway-Token"); token != "" {
		agentID, ok := a.agentsByKey[token]
		if !ok {
			return "", ErrInvalidAPIKey
		}
		return agentID, nil
	}

	// Try Basic Auth
	username, password, ok := r.BasicAuth()
	if !ok {
		return "", ErrNoCredentials
	}

	// Password is the API key, username is the agent ID (for logging purposes)
	agentID, ok := a.agentsByKey[password]
	if !ok {
		return "", ErrInvalidAPIKey
	}

	// If username was provided, verify it matches the agent ID
	if username != "" && username != agentID {
		// Username mismatch - could be intentional (agent specifying ID) or mistake
		// We trust the API key, but log this situation
		// For now, return the agent ID from the key
	}

	return agentID, nil
}
