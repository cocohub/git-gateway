// Package auth handles agent authentication via API keys.
package auth

import (
	"errors"
	"net/http"

	"github.com/cocohub/git-gateway/internal/config"
)

var (
	ErrNoCredentials       = errors.New("no credentials provided")
	ErrInvalidCredentials  = errors.New("invalid credentials")
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
			return "", ErrInvalidCredentials
		}
		return agentID, nil
	}

	// Try Basic Auth
	username, password, ok := r.BasicAuth()
	if !ok {
		return "", ErrNoCredentials
	}

	if username == "" {
		return "", ErrInvalidCredentials
	}

	// Password is the API key
	agentID, ok := a.agentsByKey[password]
	if !ok {
		return "", ErrInvalidCredentials
	}

	// Username must match the agent ID associated with the API key
	if username != agentID {
		return "", ErrInvalidCredentials
	}

	return agentID, nil
}
