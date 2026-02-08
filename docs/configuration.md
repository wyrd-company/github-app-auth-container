# Configuration

All configuration is via environment variables. There are no config files.

## Auth Service — Common

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SECRETS_BACKEND` | No | `bitwarden` | Secrets backend: `bitwarden` or `vault` |
| `GITHUB_API_URL` | No | `https://api.github.com` | GitHub API URL |
| `LOG_LEVEL` | No | `INFO` | `INFO` or `DEBUG` |
| `PORT` | No | `8080` | Server port |

## Auth Service — Bitwarden Backend

| Variable | Required | Description |
|----------|----------|-------------|
| `BW_SESSION` | Yes* | Bitwarden session token |
| `BW_SESSION_FILE` | Yes* | Path to file containing session token |
| `BW_COLLECTION_ID` | No** | Bitwarden collection ID |
| `BW_COLLECTION_ID_FILE` | No** | Path to file containing collection ID |
| `BW_SERVER_URL` | No | Bitwarden/Vaultwarden server URL |

\*Either the variable or its `_FILE` variant is required.

\*\*If omitted or set to `PERSONAL_VAULT`, loads all items from the personal vault.

## Auth Service — HashiCorp Vault Backend

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `VAULT_ADDR` | Yes | — | Vault server address |
| `VAULT_TOKEN` | Yes* | — | Vault authentication token |
| `VAULT_TOKEN_FILE` | Yes* | — | Path to file containing Vault token |
| `VAULT_MOUNT_PATH` | No | `secret` | KV v2 mount path |
| `VAULT_BASE_PATH` | No | `agents` | Base path for agent secrets |

\*Either `VAULT_TOKEN` or `VAULT_TOKEN_FILE` is required.

## Agent Containers

| Variable | Required | Description |
|----------|----------|-------------|
| `GITHUB_AUTH_SERVICE` | Yes | URL of the auth service |
| `AGENT_NAME` | Yes | Agent identifier (must match item name in secrets backend) |
| `AGENT_TOKEN` | Yes | Pre-signed authentication token |
