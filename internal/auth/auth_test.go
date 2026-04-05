package auth

import (
	"net/http"
	"testing"

	"github.com/cocohub/git-gateway/internal/config"
)

func TestAPIKeyAuthenticator(t *testing.T) {
	agents := []config.AgentConfig{
		{
			ID:      "agent-1",
			APIKeys: []string{"key-1", "key-1-backup"},
		},
		{
			ID:      "agent-2",
			APIKeys: []string{"key-2"},
		},
	}

	auth := NewAPIKeyAuthenticator(agents)

	tests := []struct {
		name      string
		setupReq  func(*http.Request)
		wantAgent string
		wantErr   error
	}{
		{
			name: "valid basic auth",
			setupReq: func(r *http.Request) {
				r.SetBasicAuth("agent-1", "key-1")
			},
			wantAgent: "agent-1",
			wantErr:   nil,
		},
		{
			name: "valid basic auth with backup key",
			setupReq: func(r *http.Request) {
				r.SetBasicAuth("agent-1", "key-1-backup")
			},
			wantAgent: "agent-1",
			wantErr:   nil,
		},
		{
			name: "valid X-Gateway-Token",
			setupReq: func(r *http.Request) {
				r.Header.Set("X-Gateway-Token", "key-2")
			},
			wantAgent: "agent-2",
			wantErr:   nil,
		},
		{
			name: "invalid key",
			setupReq: func(r *http.Request) {
				r.SetBasicAuth("agent-1", "wrong-key")
			},
			wantAgent: "",
			wantErr:   ErrInvalidCredentials,
		},
		{
			name: "agent ID mismatch",
			setupReq: func(r *http.Request) {
				r.SetBasicAuth("wrong-agent", "key-1")
			},
			wantAgent: "",
			wantErr:   ErrInvalidCredentials,
		},
		{
			name: "empty username",
			setupReq: func(r *http.Request) {
				r.SetBasicAuth("", "key-1")
			},
			wantAgent: "",
			wantErr:   ErrInvalidCredentials,
		},
		{
			name: "no credentials",
			setupReq: func(r *http.Request) {
				// No auth setup
			},
			wantAgent: "",
			wantErr:   ErrNoCredentials,
		},
		{
			name: "X-Gateway-Token takes precedence",
			setupReq: func(r *http.Request) {
				r.SetBasicAuth("agent-1", "key-1")
				r.Header.Set("X-Gateway-Token", "key-2")
			},
			wantAgent: "agent-2", // Header takes precedence
			wantErr:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/test", nil)
			tt.setupReq(req)

			agent, err := auth.Authenticate(req)

			if err != tt.wantErr {
				t.Errorf("Authenticate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if agent != tt.wantAgent {
				t.Errorf("Authenticate() agent = %q, want %q", agent, tt.wantAgent)
			}
		})
	}
}
