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
│  │ Secrets Backend  │  (secrets fetched at startup,        │
│  │ Bitwarden or     │   session closed immediately)        │
│  │ HashiCorp Vault  │                                      │
│  └──────────────────┘                                      │
└─────────────────────────────────────────────────────────────┘
```

## Features

- **GitHub App Installation Tokens**: Agents request tokens for GitHub API/git operations
- **SSH Commit Signing**: Sign commits using the same private key as the GitHub App
- **Token-Based Auth**: Agents authenticate with pre-signed tokens (no Docker socket needed)
- **Pluggable Secrets Backend**: Store credentials in Bitwarden/Vaultwarden or HashiCorp Vault
- **Fetch-at-Startup**: Secrets loaded once, backend session closed immediately
- **Helper Binaries**: Static Go binaries for git credential/signing helpers

## Quick Start

### 1. Create a GitHub App

1. Go to [GitHub Settings > Developer settings > GitHub Apps](https://github.com/settings/apps/new)
2. Create an app with required permissions (e.g., `contents: write` for pushing)
3. Generate and download a private key
4. Install the app on your repository/organization
5. Note the App ID and Installation ID

### 2. Store Credentials

Choose a secrets backend — Bitwarden/Vaultwarden or HashiCorp Vault.

Each agent needs these fields:

| Field | Value |
|-------|-------|
| `app_id` | Your GitHub App ID |
| `installation_id` | Installation ID |
| `agent_token` | Pre-signed token (see below) |
| `identity_name` | Git commit author name |
| `identity_email` | Git commit author email |
| `private_key` | PEM-encoded private key |

**Generate agent token:**
```bash
# Sign the agent name with the private key
echo -n "agent-name" | openssl dgst -sha256 -sign private-key.pem | base64 | tr -d '\n'
```

#### Option A: Bitwarden / Vaultwarden

Create a Bitwarden item in a collection with the fields above as custom fields. The private key can also be stored as an attachment named `private-key.pem`.

#### Option B: HashiCorp Vault

Store each agent as a KV v2 secret at `secret/agents/<agent-name>`:

```bash
vault kv put secret/agents/my-agent \
    app_id=123456 \
    installation_id=78901234 \
    agent_token=$(echo -n "my-agent" | openssl dgst -sha256 -sign key.pem | base64 | tr -d '\n') \
    private_key=@private-key.pem \
    identity_name="My Agent" \
    identity_email="my-agent@example.com"
```

### 3. Run the Auth Service

**With Bitwarden:**
```yaml
services:
  github-auth-service:
    image: ghcr.io/youruser/github-app-auth-container:latest
    environment:
      - BW_SESSION=${BW_SESSION}
      - BW_COLLECTION_ID=${BW_COLLECTION_ID}
      - BW_SERVER_URL=${BW_SERVER_URL:-}  # Optional: for Vaultwarden
    networks:
      - agent-network
```

**With HashiCorp Vault:**
```yaml
services:
  github-auth-service:
    image: ghcr.io/youruser/github-app-auth-container:latest
    environment:
      - SECRETS_BACKEND=vault
      - VAULT_ADDR=https://vault.example.com
      - VAULT_TOKEN=${VAULT_TOKEN}
      # - VAULT_MOUNT_PATH=secret    # default
      # - VAULT_BASE_PATH=agents     # default
    networks:
      - agent-network
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

## Documentation

- [Setting Up HashiCorp Vault](docs/vault.md) — configuring Vault as the secrets backend
- [API Reference](docs/api.md) — all REST endpoints with request/response examples
- [Configuration](docs/configuration.md) — environment variables for the auth service and agent containers
- [Helper Binaries](docs/helpers.md) — git credential helper, SSH signing, and GitHub CLI wrapper

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, building, and testing instructions.

## Security Considerations

- **Network Isolation**: The auth service should only be accessible from trusted agent containers via a private Docker network
- **Token Verification**: Agent tokens are cryptographically signed with the GitHub App private key
- **No Persistent Sessions**: Backend sessions are closed immediately after loading secrets
- **Secrets in Memory**: Private keys are held in memory only, never written to disk in the container

## License

MIT
