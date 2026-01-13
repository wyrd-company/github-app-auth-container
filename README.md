# github-app-auth-container

A REST API service that provides GitHub App authentication tokens and SSH commit signing to AI/automation agent containers. Each agent gets its own GitHub App identity for attribution.

## Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     Docker Network                          │
│                                                             │
│  ┌──────────────────┐    ┌──────────────────┐              │
│  │ github-auth-svc  │◄───│ agent-claude-1   │              │
│  │   (REST API)     │◄───│ agent-claude-2   │              │
│  │                  │◄───│ agent-...        │              │
│  └────────┬─────────┘    └──────────────────┘              │
│           │                                                 │
│           ▼                                                 │
│  ┌──────────────────┐                                      │
│  │   Bitwarden CLI  │  (secrets fetched at startup,        │
│  │                  │   session closed immediately)        │
│  └──────────────────┘                                      │
└─────────────────────────────────────────────────────────────┘
```

## Features

- **GitHub App Installation Tokens**: Agents request tokens for GitHub API/git operations
- **SSH Commit Signing**: Sign commits using the same private key as the GitHub App
- **Token-Based Auth**: Agents authenticate with pre-signed tokens (no Docker socket needed)
- **Secrets from Bitwarden**: All credentials stored in a Bitwarden collection
- **Fetch-at-Startup**: Secrets loaded once, Bitwarden session closed immediately
- **Helper Binaries**: Static Go binaries for git credential/signing helpers

## Quick Start

### 1. Create a GitHub App

1. Go to [GitHub Settings > Developer settings > GitHub Apps](https://github.com/settings/apps/new)
2. Create an app with required permissions (e.g., `contents: write` for pushing)
3. Generate and download a private key
4. Install the app on your repository/organization
5. Note the App ID and Installation ID

### 2. Store Credentials in Bitwarden

Create a Bitwarden item in a collection with:

**Custom Fields:**
| Field | Value |
|-------|-------|
| `app_id` | Your GitHub App ID |
| `installation_id` | Installation ID |
| `agent_token` | Pre-signed token (see below) |
| `identity_name` | Git commit author name |
| `identity_email` | Git commit author email |
| `private_key` | PEM-encoded private key (option 1) |

**Attachments (alternative to field):**
| File | Contents |
|------|----------|
| `private-key.pem` | GitHub App private key (option 2) |

> **Note:** You can store the private key either as a custom field (`private_key`) or as an attachment. The field option is simpler; the attachment option keeps the key separate from other metadata.

**Generate agent token:**
```bash
# Sign the agent name with the private key
echo -n "agent-name" | openssl dgst -sha256 -sign private-key.pem | base64 | tr -d '\n'
```

### 3. Run the Auth Service

```yaml
# docker-compose.yml
services:
  github-auth-service:
    image: ghcr.io/youruser/github-app-auth-container:latest
    environment:
      - BW_SESSION=${BW_SESSION}
      - BW_COLLECTION_ID=${BW_COLLECTION_ID}
      - BW_SERVER_URL=${BW_SERVER_URL:-}  # Optional: for Vaultwarden
    networks:
      - agent-network

networks:
  agent-network:
    driver: bridge
```

### 4. Configure Agent Containers

```dockerfile
# In your agent Dockerfile
FROM ghcr.io/youruser/github-app-auth-container-helpers:latest AS helpers

FROM python:3.12-slim
COPY --from=helpers /usr/local/bin/git-* /usr/local/bin/

RUN apt-get update && apt-get install -y git && \
    git config --system credential.helper github-app && \
    git config --system gpg.format ssh && \
    git config --system gpg.ssh.program git-ssh-sign && \
    git config --system commit.gpgsign true
```

```yaml
# docker-compose.yml
services:
  my-agent:
    build: .
    environment:
      - GITHUB_AUTH_SERVICE=http://github-auth-service:8080
      - AGENT_NAME=my-agent
      - AGENT_TOKEN=${MY_AGENT_TOKEN}
    networks:
      - agent-network
```

## API Reference

### Health Check

```
GET /health
```

No authentication required.

**Response:**
```json
{
  "status": "healthy",
  "agent_count": 3,
  "version": "1.0.0"
}
```

### Get Installation Token

```
GET /api/v1/token
POST /api/v1/token
```

**Headers:**
- `X-Agent-Name`: Agent identifier
- `X-Agent-Token`: Pre-signed authentication token

**Optional POST body** (for scoped tokens):
```json
{
  "repositories": ["repo-name"],
  "permissions": {"contents": "read"}
}
```

**Response:**
```json
{
  "token": "ghs_xxxxxxxxxxxx",
  "expires_at": "2024-01-15T12:00:00Z",
  "permissions": {"contents": "write"},
  "repository_selection": "all"
}
```

### Git Credentials

```
POST /api/v1/git-credentials
```

**Request:**
```json
{
  "protocol": "https",
  "host": "github.com",
  "path": "owner/repo"
}
```

**Response:**
```json
{
  "protocol": "https",
  "host": "github.com",
  "username": "x-access-token",
  "password": "ghs_xxxxxxxxxxxx"
}
```

### Identity

```
GET /api/v1/identity
```

**Response:**
```json
{
  "agent_name": "agent-claude-1",
  "identity": {
    "name": "Claude Agent 1",
    "email": "claude-1@users.noreply.github.com"
  },
  "ssh_key_id": "SHA256:xxxxxxxx"
}
```

### SSH Sign

```
POST /api/v1/ssh/sign
```

**Request:**
```json
{
  "data": "data to sign",
  "namespace": "git"
}
```

**Response:**
```json
{
  "signature": "-----BEGIN SSH SIGNATURE-----\n...",
  "key_id": "SHA256:xxxxxxxx",
  "signer_identity": {
    "name": "Claude Agent 1",
    "email": "claude-1@users.noreply.github.com"
  }
}
```

### SSH Public Key

```
GET /api/v1/ssh/public-key
```

**Response:**
```json
{
  "public_key": "ssh-rsa AAAAB3...",
  "key_id": "SHA256:xxxxxxxx",
  "identity": {
    "name": "Claude Agent 1",
    "email": "claude-1@users.noreply.github.com"
  }
}
```

## Environment Variables

### Auth Service

| Variable | Required | Description |
|----------|----------|-------------|
| `BW_SESSION` | Yes* | Bitwarden session token |
| `BW_SESSION_FILE` | Yes* | Path to file containing session token |
| `BW_COLLECTION_ID` | No** | Bitwarden collection ID |
| `BW_COLLECTION_ID_FILE` | No** | Path to file containing collection ID |
| `BW_SERVER_URL` | No | Bitwarden/Vaultwarden server URL |
| `GITHUB_API_URL` | No | GitHub API URL (default: https://api.github.com) |
| `LOG_LEVEL` | No | `INFO` or `DEBUG` (default: INFO) |
| `PORT` | No | Server port (default: 8080) |

*Either the variable or its `_FILE` variant is required.

**If omitted or set to `PERSONAL_VAULT`, the service will load all items from the personal vault instead of a specific collection. This is useful for Vaultwarden setups where organization/collection creation is limited.

### Agent Containers

| Variable | Required | Description |
|----------|----------|-------------|
| `GITHUB_AUTH_SERVICE` | Yes | URL of the auth service |
| `AGENT_NAME` | Yes | Agent identifier (must match Bitwarden item name) |
| `AGENT_TOKEN` | Yes | Pre-signed authentication token |

## Helper Binaries

The helpers image (`github-app-auth-container-helpers`) contains static Go binaries:

| Binary | Purpose |
|--------|---------|
| `git-credential-github-app` | Git credential helper |
| `git-ssh-sign` | SSH signing program for git |
| `gh-github-app` | GitHub CLI wrapper with automatic auth |

### Git Configuration

```bash
git config --global credential.helper github-app
git config --global gpg.format ssh
git config --global gpg.ssh.program git-ssh-sign
```

### GitHub CLI Wrapper

The `gh-github-app` binary wraps the GitHub CLI, automatically fetching a token from the auth service:

```bash
# Use directly
gh-github-app repo list
gh-github-app pr create --title "Fix bug" --body "Description"

# Or alias it
alias gh='gh-github-app'
gh issue list
```

Note: The `gh` CLI must be installed separately in your agent container.

## Development

### Prerequisites

- Go 1.22+
- Docker & Docker Compose
- A Bitwarden/Vaultwarden account

### Running Tests

```bash
# Copy and configure test environment
cp .env.example .env
# Edit .env with your test credentials

# Run e2e tests with Vaultwarden
./scripts/run-e2e-tests.sh
```

### Building

```bash
# Build auth service
docker build -t github-app-auth-container .

# Build helpers
docker build -f Dockerfile.helpers -t github-app-auth-container-helpers .
```

### Publishing

Tag with a semantic version to trigger the publish workflow:

```bash
git tag 1.0.0
git push origin 1.0.0
```

## Security Considerations

- **Network Isolation**: The auth service should only be accessible from trusted agent containers via a private Docker network
- **Token Verification**: Agent tokens are cryptographically signed with the GitHub App private key
- **No Persistent Sessions**: Bitwarden session is closed immediately after loading secrets
- **Secrets in Memory**: Private keys are held in memory only, never written to disk in the container

## License

MIT
