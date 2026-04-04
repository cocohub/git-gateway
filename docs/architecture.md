# Architecture

## Request Flow

```mermaid
sequenceDiagram
    participant Agent
    participant Gateway
    participant GitHub

    Agent->>Gateway: git clone http://agent:key@gateway/github.com/owner/repo.git
    
    Gateway->>Gateway: Authenticate (verify API key)
    Gateway->>Gateway: Check policy (repo + operation allowed?)
    
    alt Denied
        Gateway-->>Agent: 403 Forbidden
    else Allowed
        Gateway->>GitHub: Forward request + inject token
        GitHub-->>Gateway: Response (refs/pack data)
        Gateway-->>Agent: Response
    end
```

## Component Overview

```mermaid
flowchart LR
    A[Agent] -->|1. Request with API key| B[Gateway]
    B -->|2. Auth| C{Valid?}
    C -->|No| D[401 Unauthorized]
    C -->|Yes| E{Policy OK?}
    E -->|No| F[403 Forbidden]
    E -->|Yes| G[Inject upstream token]
    G -->|3. Proxy| H[GitHub/GitLab]
    H -->|4. Response| B
    B -->|5. Response| A
```
