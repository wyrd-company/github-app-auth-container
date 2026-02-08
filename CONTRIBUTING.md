# Contributing

## Prerequisites

- Go 1.23+
- Docker & Docker Compose
- A Bitwarden/Vaultwarden account or HashiCorp Vault server

## Building

```bash
# Build auth service
docker build -t github-app-auth-container .

# Build helpers
docker build -f Dockerfile.helpers -t github-app-auth-container-helpers .
```

## Running Tests

```bash
# Copy and configure test environment
cp .env.example .env
# Edit .env with your test credentials

# Run unit tests
go test -v ./internal/...

# Run e2e tests with Bitwarden backend
make test-e2e

# Run e2e tests with HashiCorp Vault backend
make test-e2e-vault
```

Tests requiring real GitHub App credentials (`TestTokenEndpoint`, `TestGitCredentialsEndpoint`, `TestFullGitWorkflow`) are skipped unless `TEST_GITHUB_API_ENABLED=true` is set and valid credentials are configured in `.env`.

## Publishing

Tag with a semantic version to trigger the publish workflow:

```bash
git tag 1.0.0
git push origin 1.0.0
```
