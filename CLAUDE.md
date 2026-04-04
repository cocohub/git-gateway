# Version Control Agent Gateway

A Git HTTP smart protocol proxy that enforces access control for AI agents. Agents clone/push through the gateway, which authenticates them, enforces policies, then proxies to the real upstream with injected credentials.

## Quick Start

```bash
# Setup
cp .env.example .env           # Add your tokens
cp configs/gateway.example.yaml configs/gateway.yaml  # Configure agents

# Run
make build
pm2 start bin/gateway --name gateway --cwd $(pwd) -- -config configs/gateway.yaml

# Test
git clone http://agent-id:api-key@localhost:8080/github.com/owner/repo.git
```

## Architecture

```
Agent → Gateway (auth + policy check) → Upstream (GitHub/GitLab/etc)
```

The gateway:
1. Parses the git URL to extract repo (e.g., `github.com/owner/repo`)
2. Authenticates agent via Basic Auth (password = API key) or `X-Gateway-Token` header
3. Checks policy: is this agent allowed this operation on this repo?
4. For push: parses pkt-line commands to enforce branch rules
5. Proxies to upstream with injected token

## Key Files

- `cmd/gateway/main.go` - Entry point, signal handling, hot-reload setup
- `internal/config/manager.go` - Config hot-reload via fsnotify
- `internal/proxy/handler.go` - Main HTTP handler, git protocol routing
- `internal/policy/engine.go` - Access control evaluation
- `internal/gitprotocol/pktline.go` - Git pkt-line protocol parsing
- `internal/auth/auth.go` - API key authentication

## Config Format

**`.env`** (secrets, gitignored):
```
GITHUB_TOKEN=ghp_xxx
AGENT_KEY=key_xxx
```

**`configs/gateway.yaml`** (references env vars):
```yaml
upstreams:
  - match: "github.com/**"
    token: "${GITHUB_TOKEN}"
    auth_scheme: "basic"        # GitHub requires basic auth for git ops
    username: "x-access-token"

agents:
  - id: "agent"
    api_keys: ["${AGENT_KEY}"]
    policies:
      - repos: ["github.com/owner/repo"]
        allow: [fetch, push]  # fetch = clone/fetch/pull
        branch_rules:
          deny_push: ["refs/heads/main", "refs/tags/**"]
```

## Important Implementation Details

### GitHub Auth
GitHub git operations require **basic auth**, not bearer tokens:
```yaml
auth_scheme: "basic"
username: "x-access-token"
```

### Repo Path Normalization
The gateway strips `.git` suffix for policy matching, so config can use either:
- `github.com/owner/repo`
- `github.com/owner/repo.git`

### Hot-Reload
Config reloads automatically when the file changes, or manually via:
```bash
pm2 sendSignal SIGHUP gateway
```

### Atomic Config Updates
Uses `sync/atomic.Pointer` to swap config without locks. In-flight requests continue with old config; new requests use new config.

## Common Commands

```bash
# Development
go build ./...
go test ./...
make dev                        # Run with example config

# PM2 Management
pm2 logs gateway               # View logs
pm2 restart gateway            # Restart
pm2 sendSignal SIGHUP gateway  # Reload config
pm2 delete gateway             # Stop

# Testing clone through gateway
git clone http://agent:key@localhost:8080/github.com/owner/repo.git
```

## Debugging

If clone fails with "invalid credentials":
1. Check gateway logs: `pm2 logs gateway`
2. Verify agent API key matches between `.env` and git URL password
3. Verify repo pattern in config matches the clone URL
4. Verify upstream token has access to the repo

If upstream returns 401:
- GitHub: ensure `auth_scheme: "basic"` and `username: "x-access-token"`
- Check token hasn't expired

## Testing Policy Changes

1. Edit `configs/gateway.yaml`
2. Wait 1-2 seconds for auto-reload (or `pm2 sendSignal SIGHUP gateway`)
3. Check logs for "config reloaded successfully"
4. Test with git clone/push
