# AGENTS.md

This file provides context for AI coding assistants working with this codebase.

## Project Overview

This is a Go REST API service that provides GitHub App authentication and SSH commit signing to other containers (typically AI agents). It acts as a centralized auth service so that multiple agent containers can share GitHub App credentials without each needing direct access to secrets.

## Architecture

```
Agent Container                    Auth Service                 Bitwarden
     │                                  │                           │
     │  X-Agent-Name + X-Agent-Token    │                           │
     ├─────────────────────────────────►│                           │
     │                                  │   (startup only)          │
     │                                  │◄──────────────────────────┤
     │         Installation Token       │                           │
     │◄─────────────────────────────────┤                           │
     │                                  │                           │
```

**Key design decisions:**
- **Token-based auth**: Agents authenticate with pre-signed tokens, not IP-based identification
- **Fetch-at-startup**: All secrets loaded from Bitwarden at startup, session closed immediately
- **SSH signing**: Uses the GitHub App's RSA private key for commit signing (not GPG)
- **Collection-based config**: Agents defined by Bitwarden collection contents, not a config file

## Directory Structure

```
├── cmd/
│   ├── server/                 # Main auth service binary
│   ├── git-credential-github-app/  # Git credential helper binary
│   ├── git-ssh-sign/           # SSH signing helper binary
│   └── gh-github-app/          # GitHub CLI wrapper binary
├── internal/
│   ├── api/                    # HTTP handlers, middleware, router
│   ├── config/                 # Environment variable loading
│   ├── github/                 # GitHub API client, JWT generation, token caching
│   ├── secrets/                # Bitwarden backend, in-memory store
│   └── ssh/                    # SSH signature generation
├── scripts/
│   └── seed-vault.sh           # Test setup: seeds Vaultwarden with test data
├── tests/
│   └── e2e_test.go             # End-to-end tests
└── examples/
    └── agent/                  # Example agent Dockerfile
```

## Key Files

| File | Purpose |
|------|---------|
| `internal/secrets/store.go` | In-memory secrets store, token verification |
| `internal/secrets/bitwarden.go` | Bitwarden CLI wrapper |
| `internal/api/middleware.go` | Token authentication middleware |
| `internal/api/handlers.go` | REST API endpoint handlers |
| `internal/github/service.go` | GitHub JWT generation and token fetching |
| `internal/ssh/signer.go` | SSH signature generation (SSHSIG format) |

## API Endpoints

All `/api/v1/*` endpoints require `X-Agent-Name` and `X-Agent-Token` headers.

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/health` | GET | Health check (no auth) |
| `/api/v1/token` | GET/POST | Get GitHub installation token |
| `/api/v1/git-credentials` | POST | Git credential helper format |
| `/api/v1/identity` | GET | Agent identity info |
| `/api/v1/ssh/sign` | POST | Sign data with SSH key |
| `/api/v1/ssh/public-key` | GET | Get SSH public key |

## Authentication Flow

1. Agent sends request with `X-Agent-Name: my-agent` and `X-Agent-Token: <base64>`
2. Middleware looks up agent in store by name
3. Token verified: `RSA-SHA256(agent_name)` signed with agent's private key
4. If valid, agent secrets added to request context
5. Handler uses secrets to fulfill request

## Testing

E2E tests use Vaultwarden (self-hosted Bitwarden) in Docker Compose:

```bash
docker-compose -f docker-compose.test.yml up --build --abort-on-container-exit
```

The test stack:
1. `vaultwarden` - Test Bitwarden server
2. `vault-seeder` - Populates test credentials
3. `github-auth-service` - Service under test
4. `test-runner` - Executes Go tests

## Common Tasks

### Adding a new API endpoint

1. Add types to `internal/api/types.go`
2. Add handler to `internal/api/handlers.go`
3. Register route in `internal/api/router.go`
4. Add test in `tests/e2e_test.go`

### Modifying Bitwarden item structure

1. Update `parseAgentItem()` in `internal/secrets/store.go`
2. Update `scripts/seed-vault.sh` for test seeding
3. Update README.md documentation

### Adding a new helper binary

1. Create `cmd/<binary-name>/main.go`
2. Add build step to `Dockerfile.helpers`
3. Update example agent Dockerfile

## Dependencies

- `github.com/go-chi/chi/v5` - HTTP router
- `github.com/golang-jwt/jwt/v5` - JWT generation for GitHub App auth
- `golang.org/x/crypto/ssh` - SSH key handling and signatures

## Environment Variables

The service reads all config from environment variables (no config files):

- `BW_SESSION` / `BW_SESSION_FILE` - Bitwarden session token
- `BW_COLLECTION_ID` / `BW_COLLECTION_ID_FILE` - Collection containing agent items
- `BW_SERVER_URL` - Optional Vaultwarden URL
- `GITHUB_API_URL` - Optional GitHub Enterprise URL
- `LOG_LEVEL` - INFO or DEBUG
- `PORT` - Server port (default 8080)

## Notes for AI Assistants

- This codebase does NOT use Viper or config files - just `os.Getenv()`
- There is no Docker socket access - authentication is token-based
- GPG is NOT used - SSH signing with the GitHub App's RSA key
- The Bitwarden CLI is called via `exec.Command`, not a Go library
- Secrets are loaded once at startup and held in memory
