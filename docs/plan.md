# Version Control Agent Gateway — Implementation Plan

## Context

Build a Git HTTP smart protocol proxy in Go that acts as an access control gateway for AI agents. Agents point their git remote at the gateway (e.g., `http://agent_id:agent_api_key@gateway:8080/github.com/owner/repo`), and the gateway authenticates the agent, enforces policies (repo, operation, branch level), then proxies to the real upstream with injected credentials. Provider-agnostic, config-file-driven.

## Project Structure

```
git-gateway/
├── cmd/gateway/main.go
├── internal/
│   ├── config/config.go          # YAML parsing, env var expansion, types
│   ├── auth/auth.go              # Agent auth (Basic Auth / X-Gateway-Token)
│   ├── policy/
│   │   ├── engine.go             # Allow/deny evaluation (repo, op, branch)
│   │   ├── types.go              # Decision, Operation types
│   │   └── matcher.go            # Glob matching for repos, branches
│   ├── gitprotocol/
│   │   ├── pktline.go            # pkt-line reader/writer
│   │   ├── receivepack.go        # Parse ref update commands from receive-pack body
│   │   └── types.go              # RefUpdate, ServiceType
│   ├── proxy/
│   │   ├── handler.go            # Main HTTP handler, URL routing
│   │   ├── discovery.go          # info/refs proxy
│   │   └── transport.go          # Upstream token injection
│   ├── middleware/logging.go     # Structured access logging
├── gateway.example.yaml
├── go.mod
└── Makefile
```

## Config Schema (YAML)

```yaml
server:
  listen: ":8080"
  read_timeout: 30s
  write_timeout: 120s

upstreams:
  - match: "github.com/*"        # glob on host/path
    token: "${GITHUB_TOKEN}"      # env var expansion
    auth_scheme: "bearer"         # or "basic"

agents:
  - id: "agent-codegen"
    api_keys: ["${AGENT_KEY}"]
    policies:
      - repos: ["github.com/acme/frontend", "github.com/acme/backend"]
        allow: [fetch, clone, push]
        branch_rules:
          allow_push: ["refs/heads/agent/*", "refs/heads/feature/*"]
          deny_push: ["refs/heads/main", "refs/tags/*"]   # deny wins over allow
```

## Request Flow

1. Agent sends git request to `http://gateway:8080/github.com/owner/repo.git/info/refs?service=git-upload-pack`
2. Gateway parses URL → extracts repo identifier (`github.com/owner/repo.git`) and git endpoint
3. Authenticates agent from Basic Auth (user=agent-id, pass=api-key) or `X-Gateway-Token`
4. Policy engine checks: is this agent allowed this operation on this repo?
5. For push (`git-receive-pack`): parse pkt-line commands to extract ref updates → check branch rules
6. If allowed: proxy request to `https://github.com/owner/repo.git/...` with injected upstream token
7. Log the decision (agent, repo, operation, allowed/denied, reason)

## Branch-Level Enforcement (Push)

Parse the `POST .../git-receive-pack` body which contains pkt-line formatted ref update commands:
- Each command: `old-sha SP new-sha SP refname` (first line includes `\0capabilities`)
- Read command lines via pkt-line reader, then stream the PACK data through untouched
- Use `io.TeeReader` to buffer only the small command portion (~1KB), keeping PACK data streaming
- Evaluate each refname against `allow_push`/`deny_push` glob patterns (deny wins)

## Implementation Phases

### Phase 1: Core Proxy (MVP)
1. `go mod init`, project scaffolding
2. `internal/config` — YAML parsing with `gopkg.in/yaml.v3`, env var expansion, validation
3. `internal/auth` — API key lookup from Basic Auth or header
4. `internal/policy` — repo-level + operation-level checks, glob matching
5. `internal/proxy` — URL parsing, info/refs discovery proxy, upload-pack proxy, receive-pack passthrough
6. `cmd/gateway/main.go` — wire everything, start HTTP server with graceful shutdown
7. `configs/gateway.example.yaml`
8. Tests: config parsing, auth, policy engine, URL parsing, integration test with httptest

### Phase 2: Branch-Level Enforcement
1. `internal/gitprotocol/pktline.go` — pkt-line reader/writer
2. `internal/gitprotocol/receivepack.go` — parse ref update commands, reconstruct body for forwarding
3. `internal/policy` — add branch rule evaluation
4. `internal/proxy/handler.go` — integrate command parsing into `handleReceivePack`
5. Tests: pkt-line parsing, receive-pack command extraction, branch policy evaluation

### Phase 3: Hardening
1. Structured JSON logging with `slog`
2. Request ID propagation
3. Config validation at startup (reject invalid patterns, missing tokens)
4. Makefile (build, test, lint)

## Verification

1. **Unit tests**: `go test ./...` after each phase
2. **Manual integration test**: 
   - Start gateway with example config pointing at a real GitHub repo
   - `git clone http://agent_id:agent_api_key@localhost:8080/github.com/owner/repo`
   - Attempt push to allowed branch (should succeed)
   - Attempt push to denied branch (should fail with clear error)
3. **End-to-end test**: Go test that starts a mock upstream via `httptest.Server`, launches the gateway, and issues real git HTTP protocol requests
