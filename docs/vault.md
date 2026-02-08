# Setting Up HashiCorp Vault

This guide walks through configuring HashiCorp Vault as the secrets backend.

## Prerequisites

- A running Vault server with the KV v2 secrets engine enabled
- A Vault token with read/list access to the agent secrets path
- The `vault` CLI (for seeding secrets)

For local development, you can run a dev server:

```bash
vault server -dev -dev-root-token-id=my-root-token
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=my-root-token
```

The dev server automatically enables a KV v2 engine at `secret/`.

## Secret Structure

Each agent is stored as a KV v2 secret at `<mount>/<base_path>/<agent-name>`. With the defaults (`secret` mount, `agents` base path), the path is:

```
secret/agents/<agent-name>
```

Required fields:

| Field | Description |
|-------|-------------|
| `app_id` | GitHub App ID |
| `installation_id` | GitHub App Installation ID |
| `agent_token` | RSA-SHA256 signature of the agent name, base64-encoded |
| `private_key` | PEM-encoded RSA private key (newlines escaped as `\n`) |
| `identity_name` | Git commit author name |
| `identity_email` | Git commit author email |

## Creating Agent Secrets

```bash
# 1. Generate the agent token (sign agent name with the App's private key)
AGENT_TOKEN=$(echo -n "my-agent" | openssl dgst -sha256 -sign private-key.pem | base64 | tr -d '\n')

# 2. Escape newlines in the private key for Vault storage
PRIVATE_KEY=$(awk '{printf "%s\\n", $0}' private-key.pem)

# 3. Write the secret to Vault
vault kv put secret/agents/my-agent \
    app_id="123456" \
    installation_id="78901234" \
    agent_token="${AGENT_TOKEN}" \
    private_key="${PRIVATE_KEY}" \
    identity_name="My Agent" \
    identity_email="my-agent@users.noreply.github.com"

# 4. Verify it was written
vault kv get secret/agents/my-agent
```

To add more agents, repeat for each agent name. They can share the same GitHub App (same `app_id`, `installation_id`, and `private_key`) but should have unique names and tokens.

## Vault Token Policy

If you're not using the root token, create a policy with read and list access:

```hcl
# vault-policy.hcl
path "secret/data/agents/*" {
  capabilities = ["read"]
}

path "secret/metadata/agents/*" {
  capabilities = ["read", "list"]
}
```

```bash
vault policy write github-auth vault-policy.hcl
vault token create -policy=github-auth
```

## Running the Auth Service with Vault

```yaml
services:
  github-auth-service:
    image: ghcr.io/youruser/github-app-auth-container:latest
    environment:
      - SECRETS_BACKEND=vault
      - VAULT_ADDR=https://vault.example.com
      - VAULT_TOKEN=${VAULT_TOKEN}
      # - VAULT_TOKEN_FILE=/run/secrets/vault-token  # alternative: read from file
      # - VAULT_MOUNT_PATH=secret                    # default
      # - VAULT_BASE_PATH=agents                     # default
    networks:
      - agent-network
```

The service connects to Vault at startup, loads all agent secrets from the configured path, then operates entirely from memory. The Vault token is only used during startup.

## Custom Mount Path

If your KV v2 engine is mounted at a non-default path:

```bash
# Enable KV v2 at a custom path
vault secrets enable -path=github-apps kv-v2

# Write secrets there
vault kv put github-apps/agents/my-agent app_id=... ...

# Configure the auth service
# VAULT_MOUNT_PATH=github-apps
```
