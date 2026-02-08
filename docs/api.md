# API Reference

All `/api/v1/*` endpoints require `X-Agent-Name` and `X-Agent-Token` headers.

## Health Check

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

## Get Installation Token

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

## Git Credentials

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

## Identity

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

## SSH Sign

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

## SSH Public Key

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
