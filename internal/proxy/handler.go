// Package proxy implements the Git HTTP smart protocol proxy.
package proxy

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/cocohub/git-gateway/internal/auth"
	"github.com/cocohub/git-gateway/internal/config"
	"github.com/cocohub/git-gateway/internal/gitprotocol"
	"github.com/cocohub/git-gateway/internal/policy"
)

// gatewayState holds the hot-reloadable components.
type gatewayState struct {
	auth      auth.Authenticator
	policy    *policy.Engine
	upstreams []config.Upstream
}

// Gateway is the main HTTP handler for the git proxy.
type Gateway struct {
	state  atomic.Pointer[gatewayState]
	client *http.Client
	logger *slog.Logger
	scheme string // "https" (default) or "http" for testing
}

// NewGateway creates a new gateway handler.
func NewGateway(
	authenticator auth.Authenticator,
	policyEngine *policy.Engine,
	upstreams []config.Upstream,
	logger *slog.Logger,
) *Gateway {
	g := &Gateway{
		client: &http.Client{},
		logger: logger,
	}
	g.state.Store(&gatewayState{
		auth:      authenticator,
		policy:    policyEngine,
		upstreams: upstreams,
	})
	return g
}

// UpdateConfig atomically updates the gateway's auth, policy, and upstreams.
func (g *Gateway) UpdateConfig(authenticator auth.Authenticator, policyEngine *policy.Engine, upstreams []config.Upstream) {
	g.state.Store(&gatewayState{
		auth:      authenticator,
		policy:    policyEngine,
		upstreams: upstreams,
	})
	g.logger.Info("gateway config updated")
}

// SetHTTPClient sets a custom HTTP client (useful for testing).
func (g *Gateway) SetHTTPClient(client *http.Client) {
	g.client = client
}

// SetScheme sets the upstream URL scheme (useful for testing with HTTP).
func (g *Gateway) SetScheme(scheme string) {
	g.scheme = scheme
}

// ParsedRequest contains parsed information from a git HTTP request.
type ParsedRequest struct {
	Repo     string                  // e.g. "github.com/owner/repo.git"
	Host     string                  // e.g. "github.com"
	RepoPath string                  // e.g. "owner/repo.git"
	Endpoint string                  // e.g. "/info/refs" or "/git-receive-pack"
	Service  gitprotocol.ServiceType // derived from query param or endpoint
}

// ServeHTTP handles incoming git requests.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get current state (atomic read)
	state := g.state.Load()

	// Parse the request
	parsed, err := g.parseRequest(r)
	if err != nil {
		g.logger.Warn("failed to parse request", "error", err, "path", r.URL.Path)
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Authenticate the agent
	agentID, err := state.auth.Authenticate(r)
	if err != nil {
		g.logger.Warn("authentication failed", "error", err, "repo", parsed.Repo)
		w.Header().Set("WWW-Authenticate", `Basic realm="Git Gateway"`)
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Determine operation type
	op := g.serviceToOperation(parsed.Service, r.Method)

	// Check base policy (repo + operation level)
	decision := state.policy.CheckOperation(agentID, parsed.Repo, op)
	if !decision.Allowed {
		g.logger.Warn("access denied",
			"agent", agentID,
			"repo", parsed.Repo,
			"operation", op,
			"reason", decision.Reason,
		)
		http.Error(w, "Forbidden: "+decision.Reason, http.StatusForbidden)
		return
	}

	// Route to appropriate handler
	switch {
	case parsed.Endpoint == "/info/refs":
		g.handleDiscovery(w, r, parsed, agentID, state)
	case parsed.Endpoint == "/git-upload-pack":
		g.handleUploadPack(w, r, parsed, agentID, state)
	case parsed.Endpoint == "/git-receive-pack":
		g.handleReceivePack(w, r, parsed, agentID, state)
	default:
		http.Error(w, "Not Found", http.StatusNotFound)
	}
}

func (g *Gateway) parseRequest(r *http.Request) (*ParsedRequest, error) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	// Find the git endpoint suffix
	var endpoint string
	var repoPath string

	for _, suffix := range []string{"/info/refs", "/git-upload-pack", "/git-receive-pack"} {
		if strings.HasSuffix(path, suffix) {
			endpoint = suffix
			repoPath = strings.TrimSuffix(path, suffix)
			break
		}
	}

	if endpoint == "" {
		return nil, fmt.Errorf("unknown git endpoint in path: %s", path)
	}

	// Extract host from repo path
	parts := strings.SplitN(repoPath, "/", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid repo path: %s", repoPath)
	}

	host := parts[0]
	repoPathOnly := parts[1]

	// Determine service type
	var service gitprotocol.ServiceType
	if endpoint == "/info/refs" {
		svc := r.URL.Query().Get("service")
		switch svc {
		case "git-upload-pack":
			service = gitprotocol.ServiceUploadPack
		case "git-receive-pack":
			service = gitprotocol.ServiceReceivePack
		default:
			return nil, fmt.Errorf("unsupported or missing service: %s", svc)
		}
	} else {
		service = gitprotocol.ServiceType(strings.TrimPrefix(endpoint, "/"))
	}

	// Normalize repo path - strip .git suffix for policy matching
	// but keep original for upstream URL construction
	normalizedRepo := strings.TrimSuffix(repoPath, ".git")

	return &ParsedRequest{
		Repo:     normalizedRepo,
		Host:     host,
		RepoPath: repoPathOnly,
		Endpoint: endpoint,
		Service:  service,
	}, nil
}

func (g *Gateway) serviceToOperation(service gitprotocol.ServiceType, method string) config.Operation {
	switch service {
	case gitprotocol.ServiceUploadPack:
		return config.OpFetch // clone and fetch both use upload-pack
	case gitprotocol.ServiceReceivePack:
		return config.OpPush
	default:
		return config.OpFetch
	}
}

func (g *Gateway) handleDiscovery(w http.ResponseWriter, r *http.Request, parsed *ParsedRequest, agentID string, state *gatewayState) {
	upstreamURL := fmt.Sprintf("%s://%s/%s%s?%s",
		g.getScheme(), parsed.Host, parsed.RepoPath, parsed.Endpoint, r.URL.RawQuery)

	g.proxyRequest(w, r, upstreamURL, parsed, agentID, state)
}

func (g *Gateway) handleUploadPack(w http.ResponseWriter, r *http.Request, parsed *ParsedRequest, agentID string, state *gatewayState) {
	upstreamURL := fmt.Sprintf("%s://%s/%s%s",
		g.getScheme(), parsed.Host, parsed.RepoPath, parsed.Endpoint)

	g.proxyRequest(w, r, upstreamURL, parsed, agentID, state)
}

func (g *Gateway) handleReceivePack(w http.ResponseWriter, r *http.Request, parsed *ParsedRequest, agentID string, state *gatewayState) {
	// Parse ref update commands for branch-level enforcement
	updates, body, err := gitprotocol.ParseReceivePackCommands(r.Body)
	if err != nil {
		g.logger.Error("failed to parse receive-pack commands", "error", err)
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Convert to policy.RefUpdate
	policyUpdates := make([]policy.RefUpdate, len(updates))
	for i, u := range updates {
		policyUpdates[i] = policy.RefUpdate{
			OldSHA:  u.OldSHA,
			NewSHA:  u.NewSHA,
			RefName: u.RefName,
		}
	}

	// Check branch rules
	decision := state.policy.CheckBranches(agentID, parsed.Repo, policyUpdates)
	if !decision.Allowed {
		g.logger.Warn("push denied by branch rules",
			"agent", agentID,
			"repo", parsed.Repo,
			"reason", decision.Reason,
		)
		w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
		w.WriteHeader(http.StatusOK) // Git expects 200 with error in body
		gitprotocol.WriteReceivePackError(w, decision.Reason)
		return
	}

	g.logger.Info("push allowed",
		"agent", agentID,
		"repo", parsed.Repo,
		"refs", len(updates),
	)

	// Proxy the push
	upstreamURL := fmt.Sprintf("%s://%s/%s%s",
		g.getScheme(), parsed.Host, parsed.RepoPath, parsed.Endpoint)

	g.proxyRequestWithBody(w, r, upstreamURL, parsed, agentID, body, state)
}

func (g *Gateway) proxyRequest(w http.ResponseWriter, r *http.Request, upstreamURL string, parsed *ParsedRequest, agentID string, state *gatewayState) {
	g.proxyRequestWithBody(w, r, upstreamURL, parsed, agentID, r.Body, state)
}

func (g *Gateway) proxyRequestWithBody(w http.ResponseWriter, r *http.Request, upstreamURL string, parsed *ParsedRequest, agentID string, body io.Reader, state *gatewayState) {
	// Find matching upstream
	upstream := g.findUpstream(parsed.Repo, state)
	if upstream == nil {
		g.logger.Error("no upstream configured", "repo", parsed.Repo)
		http.Error(w, "No upstream configured for this repository", http.StatusBadGateway)
		return
	}

	// Create upstream request
	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, body)
	if err != nil {
		g.logger.Error("failed to create upstream request", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Copy relevant headers
	upstreamReq.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	upstreamReq.Header.Set("Accept", r.Header.Get("Accept"))
	upstreamReq.Header.Set("User-Agent", "git-gateway-gateway/1.0")

	// Add upstream authentication
	g.addUpstreamAuth(upstreamReq, upstream)

	// Execute request
	resp, err := g.client.Do(upstreamReq)
	if err != nil {
		g.logger.Error("upstream request failed", "error", err, "url", upstreamURL)
		http.Error(w, "Bad Gateway: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	g.logger.Info("proxied request",
		"agent", agentID,
		"repo", parsed.Repo,
		"endpoint", parsed.Endpoint,
		"upstream_status", resp.StatusCode,
	)
}

func (g *Gateway) findUpstream(repo string, state *gatewayState) *config.Upstream {
	for i := range state.upstreams {
		if policy.MatchGlob(state.upstreams[i].Match, repo) {
			return &state.upstreams[i]
		}
	}
	return nil
}

func (g *Gateway) addUpstreamAuth(req *http.Request, upstream *config.Upstream) {
	switch upstream.AuthScheme {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+upstream.Token)
	case "basic":
		username := upstream.Username
		if username == "" {
			username = "x-access-token" // GitHub Apps default
		}
		req.SetBasicAuth(username, upstream.Token)
	}
}

func (g *Gateway) getScheme() string {
	if g.scheme != "" {
		return g.scheme
	}
	return "https"
}
