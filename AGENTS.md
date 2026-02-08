# AGENTS.md

This file provides context for AI coding assistants working with this codebase.

## Project Overview

This is a Go REST API service that provides GitHub App authentication and SSH commit signing to other containers (typically AI agents). It acts as a centralized auth service so that multiple agent containers can share GitHub App credentials without each needing direct access to secrets.

## Architecture

```
Agent Container                    Auth Service           Secrets Backend
     │                                  │               (Bitwarden or Vault)
     │  X-Agent-Name + X-Agent-Token    │                       │
     ├─────────────────────────────────►│                       │
     │                                  │   (startup only)      │
     │                                  │◄──────────────────────┤
     │         Installation Token       │                       │
     │◄─────────────────────────────────┤                       │
     │                                  │                       │
```

**Key design decisions:**
- **Token-based auth**: Agents authenticate with pre-signed tokens, not IP-based identification
- **Pluggable backends**: SecretsBackend interface supports Bitwarden and HashiCorp Vault
- **Fetch-at-startup**: All secrets loaded from backend at startup, session closed immediately
- **SSH signing**: Uses the GitHub App's RSA private key for commit signing (not GPG)
- **Scope-based config**: Agents defined by collection (Bitwarden) or path (Vault) contents

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
│   ├── secrets/                # Secrets backends (Bitwarden, Vault), in-memory store
│   └── ssh/                    # SSH signature generation
├── scripts/
│   ├── seed-vault.sh           # Test setup: seeds Vaultwarden with test data
│   └── seed-vault-hcv.sh       # Test setup: seeds HashiCorp Vault with test data
├── tests/
│   └── e2e_test.go             # End-to-end tests
└── examples/
    └── agent/                  # Example agent Dockerfile
```

## Key Files

| File | Purpose |
|------|---------|
| `internal/secrets/backend.go` | SecretsBackend interface, shared types |
| `internal/secrets/store.go` | In-memory secrets store, token verification |
| `internal/secrets/bitwarden.go` | Bitwarden CLI backend |
| `internal/secrets/vault.go` | HashiCorp Vault KV v2 backend |
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

E2E tests run against both backends:

```bash
# Bitwarden backend (Vaultwarden)
make test-e2e

# HashiCorp Vault backend
make test-e2e-vault
```

The Bitwarden test stack (`docker-compose.test.yml`):
1. `vaultwarden` - Test Bitwarden server
2. `vault-seeder` - Populates test credentials
3. `github-auth-service` - Service under test
4. `test-runner` - Executes Go tests

The Vault test stack (`docker-compose.test-vault.yml`):
1. `vault-server` - HashiCorp Vault dev server
2. `vault-seeder` - Populates test credentials
3. `github-auth-service` - Service under test (SECRETS_BACKEND=vault)
4. `test-runner` - Executes Go tests (same tests, backend-agnostic)

## Common Tasks

### Adding a new API endpoint

1. Add types to `internal/api/types.go`
2. Add handler to `internal/api/handlers.go`
3. Register route in `internal/api/router.go`
4. Add test in `tests/e2e_test.go`

### Modifying agent item structure

1. Update `parseAgentItem()` in `internal/secrets/store.go`
2. Update `scripts/seed-vault.sh` (Bitwarden) and `scripts/seed-vault-hcv.sh` (Vault) for test seeding
3. Update README.md documentation

### Adding a new secrets backend

1. Implement `SecretsBackend` interface in `internal/secrets/<backend>.go`
2. Add config fields in `internal/config/config.go`
3. Add case to the backend selection switch in `cmd/server/main.go`
4. Add e2e test infrastructure (docker-compose, seeder script)
5. Update documentation

### Adding a new helper binary

1. Create `cmd/<binary-name>/main.go`
2. Add build step to `Dockerfile.helpers`
3. Update example agent Dockerfile

## Dependencies

- `github.com/go-chi/chi/v5` - HTTP router
- `github.com/golang-jwt/jwt/v5` - JWT generation for GitHub App auth
- `github.com/hashicorp/vault/api` - HashiCorp Vault client
- `golang.org/x/crypto/ssh` - SSH key handling and signatures

## Environment Variables

The service reads all config from environment variables (no config files):

- `SECRETS_BACKEND` - `bitwarden` (default) or `vault`
- `BW_SESSION` / `BW_SESSION_FILE` - Bitwarden session token (bitwarden backend)
- `BW_COLLECTION_ID` / `BW_COLLECTION_ID_FILE` - Collection containing agent items (bitwarden backend)
- `BW_SERVER_URL` - Optional Vaultwarden URL (bitwarden backend)
- `VAULT_ADDR` - Vault server address (vault backend)
- `VAULT_TOKEN` / `VAULT_TOKEN_FILE` - Vault auth token (vault backend)
- `VAULT_MOUNT_PATH` - KV v2 mount path, default `secret` (vault backend)
- `VAULT_BASE_PATH` - Base path for agents, default `agents` (vault backend)
- `GITHUB_API_URL` - Optional GitHub Enterprise URL
- `LOG_LEVEL` - INFO or DEBUG
- `PORT` - Server port (default 8080)

## Notes for AI Assistants

- This codebase does NOT use Viper or config files - just `os.Getenv()`
- There is no Docker socket access - authentication is token-based
- GPG is NOT used - SSH signing with the GitHub App's RSA key
- The Bitwarden CLI is called via `exec.Command`, not a Go library
- The Vault backend uses the `hashicorp/vault/api` Go client (no CLI needed)
- Secrets are loaded once at startup and held in memory
- Backend selection is via `SECRETS_BACKEND` env var, defaulting to `bitwarden`
