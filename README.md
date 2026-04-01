# Version Control Agent Gateway

A Git HTTP proxy that enforces fine-grained access control for AI agents. The gateway sits between AI agents and Git providers (GitHub, GitLab, etc.), authenticating agents, enforcing policies, and proxying requests with injected credentials.

## Features

- **Agent Authentication**: API key-based auth via Basic Auth or custom header
- **Repository Access Control**: Allow/deny access per agent per repository
- **Operation Control**: Restrict agents to fetch/clone/push per repo
- **Branch Protection**: Deny pushes to specific branches (e.g., `main`, `master`)
- **Path Protection**: Deny modifications to specific files (e.g., `.github/workflows/*`)
- **Provider Agnostic**: Works with GitHub, GitLab, Bitbucket, or any Git HTTP server
- **Hot-Reload**: Config changes apply without restart

## Installation

```bash
# Clone and build
git clone https://github.com/cocohub/git-gateway.git
cd git-gateway
go build -o bin/gateway ./cmd/gateway

# Or with make
make build
```

## Quick Start

1. **Create configuration files**:

```bash
cp .env.example .env
cp gateway.example.yaml gateway.yaml
```

2. **Edit `.env`** with your credentials:

```bash
# Your GitHub token (needs repo access)
GITHUB_TOKEN=ghp_xxxxxxxxxxxx

# API keys for your agents (generate with: openssl rand -hex 32)
AGENT_CODEGEN_KEY=your-secret-key-here
```

3. **Edit `gateway.yaml`** with your agent policies:

```yaml
server:
  listen: ":8080"

upstreams:
  - match: "github.com/**"
    token: "${GITHUB_TOKEN}"
    auth_scheme: "basic"
    username: "x-access-token"

agents:
  - id: "codegen-agent"
    api_keys: ["${AGENT_CODEGEN_KEY}"]
    policies:
      - repos: ["github.com/myorg/myrepo"]
        allow: [fetch, clone, push]
        branch_rules:
          deny_push:
            - "refs/heads/main"
            - "refs/heads/master"
```

4. **Start the gateway**:

```bash
./bin/gateway -config gateway.yaml
```

5. **Configure your agent** to use the gateway:

```bash
# Clone through the gateway
git clone http://codegen-agent:your-secret-key-here@localhost:8080/github.com/myorg/myrepo.git

# Or set remote on existing repo
git remote set-url origin http://codegen-agent:your-secret-key-here@localhost:8080/github.com/myorg/myrepo.git
```

## Configuration

### Environment Variables

The gateway loads `.env` automatically. Use `${VAR}` syntax in YAML to reference them:

```bash
# .env
GITHUB_TOKEN=ghp_xxxx
AGENT_KEY=secret123
```

```yaml
# gateway.yaml
token: "${GITHUB_TOKEN}"
api_keys: ["${AGENT_KEY}"]
```

### Upstreams

Configure credentials for Git providers:

```yaml
upstreams:
  # GitHub - requires basic auth for git operations
  - match: "github.com/**"
    token: "${GITHUB_TOKEN}"
    auth_scheme: "basic"
    username: "x-access-token"

  # GitLab - can use bearer token
  - match: "gitlab.com/**"
    token: "${GITLAB_TOKEN}"
    auth_scheme: "bearer"

  # Self-hosted GitLab
  - match: "git.company.com/**"
    token: "${INTERNAL_GITLAB_TOKEN}"
    auth_scheme: "bearer"
```

### Agent Policies

Define what each agent can access:

```yaml
agents:
  - id: "codegen-agent"
    api_keys: ["${AGENT_CODEGEN_KEY}"]
    policies:
      - repos:
          - "github.com/myorg/frontend"
          - "github.com/myorg/backend"
        allow:
          - fetch
          - clone
          - push
        branch_rules:
          allow_push:
            - "refs/heads/agent/**"
            - "refs/heads/feature/**"
          deny_push:
            - "refs/heads/main"
            - "refs/heads/master"
            - "refs/tags/**"
        path_rules:
          deny_modify:
            - ".github/workflows/**"
            - "Dockerfile"
            - "go.mod"

  - id: "readonly-agent"
    api_keys: ["${AGENT_READONLY_KEY}"]
    policies:
      - repos: ["github.com/myorg/**"]  # Glob pattern
        allow: [fetch, clone]
        # No push - not in allow list
```

### Policy Rules

| Rule | Description |
|------|-------------|
| `repos` | Glob patterns for repositories (e.g., `github.com/org/*`) |
| `allow` | Operations allowed: `fetch`, `clone`, `push` |
| `branch_rules.allow_push` | Refs the agent CAN push to (allowlist) |
| `branch_rules.deny_push` | Refs the agent CANNOT push to (blocklist, takes precedence) |
| `path_rules.deny_modify` | File paths that cannot be modified |
| `path_rules.allow_modify` | Only these paths can be modified (allowlist mode) |

## Running in Production

### With systemd

```ini
# /etc/systemd/system/git-gateway.service
[Unit]
Description=Git Agent Gateway
After=network.target

[Service]
Type=simple
User=gateway
WorkingDirectory=/opt/gateway
ExecStart=/opt/gateway/bin/gateway -config /opt/gateway/gateway.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=always

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable git-gateway
sudo systemctl start git-gateway

# Reload config without restart
sudo systemctl reload git-gateway
```

### With PM2

```bash
pm2 start bin/gateway --name gateway -- -config gateway.yaml

# Reload config
pm2 sendSignal SIGHUP gateway

# View logs
pm2 logs gateway
```

### With Docker

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o gateway ./cmd/gateway

FROM alpine:latest
COPY --from=builder /app/gateway /usr/local/bin/
ENTRYPOINT ["gateway"]
CMD ["-config", "/etc/gateway/gateway.yaml"]
```

## Hot-Reload

The gateway automatically reloads when the config file changes. You can also trigger a manual reload:

```bash
# Send SIGHUP
kill -HUP $(pgrep gateway)

# Or with PM2
pm2 sendSignal SIGHUP gateway

# Or with systemd
sudo systemctl reload git-gateway
```

Invalid configurations are rejected and the gateway continues with the previous config.

## Logging

Logs are JSON-formatted by default:

```json
{"time":"...","level":"INFO","msg":"proxied request","agent":"codegen","repo":"github.com/org/repo","endpoint":"/info/refs","upstream_status":200}
{"time":"...","level":"WARN","msg":"access denied","agent":"codegen","repo":"github.com/org/secret","operation":"fetch","reason":"operation fetch not allowed on repo"}
```

Configure log level and format:

```yaml
log:
  level: "info"   # debug, info, warn, error
  format: "json"  # json or text
```

## Development

```bash
# Run tests
make test

# Run with hot-reload during development
make dev

# Build
make build

# Format code
make fmt
```