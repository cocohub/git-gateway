package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cocohub/git-gateway/internal/auth"
	"github.com/cocohub/git-gateway/internal/config"
	"github.com/cocohub/git-gateway/internal/policy"
)

func TestGateway_ParseRequest(t *testing.T) {
	g := &Gateway{}

	tests := []struct {
		name     string
		path     string
		query    string
		wantRepo string
		wantHost string
		wantSvc  string
		wantErr  bool
	}{
		{
			name:     "clone discovery",
			path:     "/github.com/owner/repo.git/info/refs",
			query:    "service=git-upload-pack",
			wantRepo: "github.com/owner/repo", // .git suffix stripped for policy matching
			wantHost: "github.com",
			wantSvc:  "git-upload-pack",
		},
		{
			name:     "push discovery",
			path:     "/github.com/owner/repo.git/info/refs",
			query:    "service=git-receive-pack",
			wantRepo: "github.com/owner/repo",
			wantHost: "github.com",
			wantSvc:  "git-receive-pack",
		},
		{
			name:     "upload-pack",
			path:     "/gitlab.com/group/project.git/git-upload-pack",
			wantRepo: "gitlab.com/group/project",
			wantHost: "gitlab.com",
			wantSvc:  "git-upload-pack",
		},
		{
			name:     "receive-pack",
			path:     "/github.com/owner/repo.git/git-receive-pack",
			wantRepo: "github.com/owner/repo",
			wantHost: "github.com",
			wantSvc:  "git-receive-pack",
		},
		{
			name:    "invalid endpoint",
			path:    "/github.com/owner/repo.git/invalid",
			wantErr: true,
		},
		{
			name:    "missing service param",
			path:    "/github.com/owner/repo.git/info/refs",
			query:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := tt.path
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest("GET", url, nil)

			parsed, err := g.parseRequest(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if parsed.Repo != tt.wantRepo {
				t.Errorf("Repo = %q, want %q", parsed.Repo, tt.wantRepo)
			}
			if parsed.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", parsed.Host, tt.wantHost)
			}
			if string(parsed.Service) != tt.wantSvc {
				t.Errorf("Service = %q, want %q", parsed.Service, tt.wantSvc)
			}
		})
	}
}

func TestGateway_Authentication(t *testing.T) {
	// Create a mock upstream
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify upstream gets the token (not the agent's key)
		if auth := r.Header.Get("Authorization"); auth != "Bearer upstream-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
		w.Write([]byte("001e# service=git-upload-pack\n0000"))
	}))
	defer mockUpstream.Close()

	// Extract host from mock URL
	upstreamHost := strings.TrimPrefix(mockUpstream.URL, "http://")

	agents := []config.AgentConfig{
		{
			ID:      "test-agent",
			APIKeys: []string{"valid-key"},
			Policies: []config.Policy{{
				Repos: []string{upstreamHost + "/**"},
				Allow: []config.Operation{config.OpFetch, config.OpClone},
			}},
		},
	}

	upstreams := []config.Upstream{
		{Match: upstreamHost + "/**", Token: "upstream-token", AuthScheme: "bearer"},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gateway := NewGateway(
		auth.NewAPIKeyAuthenticator(agents),
		policy.NewEngine(agents),
		upstreams,
		logger,
	)
	gateway.SetScheme("http") // Use HTTP for testing

	tests := []struct {
		name       string
		setupAuth  func(*http.Request)
		wantStatus int
	}{
		{
			name: "valid auth",
			setupAuth: func(r *http.Request) {
				r.SetBasicAuth("test-agent", "valid-key")
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "no auth",
			setupAuth:  func(r *http.Request) {},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "invalid key",
			setupAuth: func(r *http.Request) {
				r.SetBasicAuth("test-agent", "wrong-key")
			},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET",
				"/"+upstreamHost+"/owner/repo.git/info/refs?service=git-upload-pack", nil)
			tt.setupAuth(req)

			rec := httptest.NewRecorder()
			gateway.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestGateway_PolicyEnforcement(t *testing.T) {
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockUpstream.Close()

	upstreamHost := strings.TrimPrefix(mockUpstream.URL, "http://")

	agents := []config.AgentConfig{
		{
			ID:      "read-only",
			APIKeys: []string{"read-key"},
			Policies: []config.Policy{{
				Repos: []string{upstreamHost + "/allowed/**"},
				Allow: []config.Operation{config.OpFetch, config.OpClone},
			}},
		},
	}

	upstreams := []config.Upstream{
		{Match: upstreamHost + "/**", Token: "token", AuthScheme: "bearer"},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	gateway := NewGateway(
		auth.NewAPIKeyAuthenticator(agents),
		policy.NewEngine(agents),
		upstreams,
		logger,
	)
	gateway.SetScheme("http") // Use HTTP for testing

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{
			name:       "allowed repo",
			path:       "/" + upstreamHost + "/allowed/repo.git/info/refs?service=git-upload-pack",
			wantStatus: http.StatusOK,
		},
		{
			name:       "denied repo",
			path:       "/" + upstreamHost + "/denied/repo.git/info/refs?service=git-upload-pack",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "push denied for read-only agent",
			path:       "/" + upstreamHost + "/allowed/repo.git/info/refs?service=git-receive-pack",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			req.SetBasicAuth("read-only", "read-key")

			rec := httptest.NewRecorder()
			gateway.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
