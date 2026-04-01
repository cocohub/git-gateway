# Security Analysis Report

**Date:** 2026-04-01  
**Target:** Version Control Agent Gateway  
**Scope:** Vulnerabilities exploitable by agents to bypass access controls

---

## Executive Summary

This security analysis identified **3 critical**, **2 high**, and **4 medium** severity vulnerabilities in the gateway's access control mechanisms. The most severe finding is that **path-level restrictions are completely unenforced** despite being configurable, allowing agents to modify any file in allowed repositories.

---

## Critical Findings

### 1. Path Rules Not Enforced (CRITICAL)

**Location:** `internal/proxy/handler.go`

**Description:** The `PathRules` configuration (allow_modify, deny_modify) is parsed and stored, and the policy engine has a `CheckPaths()` function, but **it is never called** in the request handling flow.

```go
// handler.go:214-257 - handleReceivePack only calls CheckBranches
decision := state.policy.CheckBranches(agentID, parsed.Repo, policyUpdates)
// CheckPaths is NEVER called
```

**Impact:** An agent with push access to a repository can modify ANY file, including:
- `.github/workflows/*` (CI/CD pipelines - supply chain attack vector)
- `Dockerfile`, `go.mod`, `package.json` (dependency injection)
- Security-sensitive configs

**Exploitation:**
```bash
# Agent configured with deny_modify: [".github/workflows/**"]
# Can still push workflow changes because path rules are not checked
git push origin feature/malicious-workflow
```

**Severity:** CRITICAL - Complete bypass of path-level access controls

---

### 2. New Branch Bypasses Path Checking (CRITICAL)

**Location:** `internal/pathcheck/checker.go:45-49`

**Description:** Even if path checking were enforced, creating a new branch returns an empty path list, allowing all files.

```go
// For new branches, oldSHA is all zeros - can't compare
if strings.HasPrefix(oldSHA, "000000") {
    // New branch - would need to compare against the default branch
    // For now, return empty (allow all) for new branches
    return []string{}, nil
}
```

**Impact:** An agent can bypass ALL path restrictions by:
1. Creating a new branch with malicious file changes
2. The path check returns empty (all allowed)
3. Another collaborator merges the branch

**Exploitation:**
```bash
# Create new branch with restricted file changes
git checkout -b agent/bypass-paths
echo "malicious" > .github/workflows/pwned.yml
git add . && git commit -m "bypass"
git push origin agent/bypass-paths  # Path check bypassed
```

**Severity:** CRITICAL - Complete path restriction bypass via new branches

---

### 3. Clone Mapped to Fetch Loses Permission Granularity (CRITICAL)

**Location:** `internal/proxy/handler.go:189-198`

**Description:** Both `clone` and `fetch` operations map to the same `OpFetch` operation type.

```go
func (g *Gateway) serviceToOperation(service gitprotocol.ServiceType, method string) config.Operation {
    switch service {
    case gitprotocol.ServiceUploadPack:
        return config.OpFetch // clone and fetch both use upload-pack
    // ...
}
```

**Impact:** If an administrator intends to allow an agent to `fetch` (update existing clones) but not `clone` (initial access), this distinction is impossible to enforce. More importantly, the `clone` operation type in config is effectively useless.

**Severity:** CRITICAL - Policy configuration does not work as documented

---

## High Severity Findings

### 4. Username Mismatch Silently Ignored (HIGH)

**Location:** `internal/auth/auth.go:70-76`

**Description:** When Basic Auth username doesn't match the agent ID derived from the API key, authentication still succeeds.

```go
// If username was provided, verify it matches the agent ID
if username != "" && username != agentID {
    // Username mismatch - could be intentional (agent specifying ID) or mistake
    // We trust the API key, but log this situation
    // For now, return the agent ID from the key
}
return agentID, nil  // Always succeeds if key is valid
```

**Impact:** 
- An agent can authenticate as any other agent by using their API key with a different username
- Audit logs show the wrong agent ID (from key, not from username)
- Agent impersonation for audit log confusion

**Exploitation:**
```bash
# Agent B's key used with Agent A's username
git clone http://agent-a:agent-b-api-key@localhost:8080/repo.git
# Authenticates as agent-b but logs may show agent-a attempted something
```

**Severity:** HIGH - Audit log pollution and potential for confusion attacks

---

### 5. No Rate Limiting on Authentication (HIGH)

**Location:** `internal/auth/auth.go`, `cmd/gateway/main.go`

**Description:** No rate limiting is implemented for authentication attempts, allowing unlimited brute-force attempts against API keys.

**Impact:** 
- Brute-force attacks against API keys
- Credential stuffing if keys follow predictable patterns
- DoS via authentication flooding

**Severity:** HIGH - API key compromise through brute force

---

## Medium Severity Findings

### 6. First-Policy-Wins Creates Unexpected Behavior (MEDIUM)

**Location:** `internal/policy/engine.go:51-58`

**Description:** When an agent has multiple policies matching a repo, only the first match is used.

```go
for i := range agent.Policies {
    if matchesRepo(repo, agent.Policies[i].Repos) {
        matchedPolicy = &agent.Policies[i]
        break  // First match wins
    }
}
```

**Impact:** Policy ordering in config is security-critical but not documented. An overly permissive policy listed first will override more restrictive policies.

**Config Example:**
```yaml
policies:
  - repos: ["github.com/**"]       # Matches first
    allow: [fetch, clone, push]     # Too permissive
  - repos: ["github.com/acme/secret"]  # Never evaluated
    allow: [fetch]
```

**Severity:** MEDIUM - Misconfiguration leads to over-permission

---

### 7. Timing Side-Channel in API Key Lookup (MEDIUM)

**Location:** `internal/auth/auth.go:51-55, 65-67`

**Description:** API key lookup uses Go map indexing which may leak timing information about key existence.

```go
agentID, ok := a.agentsByKey[token]  // Variable-time operation
```

**Impact:** Timing attacks could reveal whether a key prefix exists, helping narrow brute-force attacks.

**Severity:** MEDIUM - Facilitates brute-force attacks

---

### 8. Ref Name Not Validated for Prefix (MEDIUM)

**Location:** `internal/gitprotocol/receivepack.go:44-75`

**Description:** Ref names from git protocol are used directly without validating they start with `refs/`. The branch rule matching assumes refs follow standard format.

```go
RefName: refName,  // No validation that this starts with refs/
```

**Impact:** Malformed ref names could potentially bypass branch rules expecting `refs/heads/` or `refs/tags/` patterns.

**Exploitation Attempt:**
```
# Custom ref that doesn't match deny patterns
refs-heads-main  # Instead of refs/heads/main
```

**Severity:** MEDIUM - Potential bypass with malformed refs (needs further testing)

---

### 9. Host Extraction Trusts URL Path (MEDIUM)

**Location:** `internal/proxy/handler.go:152-158`

**Description:** The host is extracted from the first path segment without validation against a whitelist.

```go
parts := strings.SplitN(repoPath, "/", 2)
host := parts[0]  // Attacker-controlled
```

**Impact:** If an upstream pattern like `**` is configured, an agent could potentially route requests to unintended hosts.

**Severity:** MEDIUM - Depends on upstream configuration

---

## Low Severity Findings

### 10. Error Messages Leak Policy Details

Denial messages reveal policy structure:
```go
fmt.Sprintf("push to %s denied by branch rules", update.RefName)
fmt.Sprintf("operation %s not allowed on repo %s", op, repo)
```

This helps attackers understand policy configuration for targeted bypass attempts.

---

## Recommendations

### Immediate Actions (Critical)

1. **Implement path checking in push handler:**
```go
// After CheckBranches succeeds, add:
paths, err := pathChecker.GetChangedPaths(ctx, parsed.Repo, ...)
if err != nil { /* handle */ }
pathDecision := state.policy.CheckPaths(agentID, parsed.Repo, paths)
if !pathDecision.Allowed { /* deny */ }
```

2. **Fix new branch path checking:**
   - Compare against default branch instead of returning empty
   - Or require all path rules to use allowlist mode for new branches

3. **Remove unused `clone` operation or implement distinction**

### Short-Term Actions (High)

4. **Fail authentication on username mismatch** or document the behavior
5. **Add rate limiting** using a token bucket or sliding window

### Medium-Term Actions

6. **Document policy evaluation order** and consider "most restrictive wins"
7. **Use constant-time comparison** for API keys
8. **Validate ref name format** before policy evaluation
9. **Whitelist allowed hosts** in upstream configuration

---

## Attack Scenarios

### Scenario 1: Supply Chain Attack via Path Bypass

1. Agent has push access to `github.com/acme/webapp` with `deny_modify: [".github/workflows/**"]`
2. Agent pushes malicious workflow file (path rules not enforced)
3. Workflow runs on next PR, exfiltrating secrets or modifying releases

### Scenario 2: Protected Branch Bypass via New Branch

1. Agent denied push to `refs/heads/main`
2. Agent creates `refs/heads/agent/sneaky` with merge commit targeting main
3. If branch protection allows merge from any branch, main is modified indirectly

### Scenario 3: Audit Log Confusion

1. Attacker compromises Agent B's API key
2. Uses Agent A's username in Basic Auth
3. Malicious actions may be attributed to wrong agent in some log paths

---

## Conclusion

The gateway has fundamental gaps in its access control enforcement. The most critical issue is that **path-level restrictions exist only in configuration but are never enforced at runtime**. Until these issues are addressed, agents can bypass file-level restrictions entirely, making the `path_rules` feature security theater.
