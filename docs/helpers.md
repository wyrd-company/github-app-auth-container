# Helper Binaries

The helpers image (`github-app-auth-container-helpers`) contains static Go binaries that agent containers use to authenticate with GitHub via the auth service.

| Binary | Purpose |
|--------|---------|
| `git-credential-github-app` | Git credential helper |
| `git-ssh-sign` | SSH signing program for git |
| `gh-github-app` | GitHub CLI wrapper with automatic auth |

## Installation

Copy the binaries into your agent container from the helpers image:

```dockerfile
FROM ghcr.io/youruser/github-app-auth-container-helpers:latest AS helpers

FROM python:3.12-slim
COPY --from=helpers /usr/local/bin/git-* /usr/local/bin/
COPY --from=helpers /usr/local/bin/gh-* /usr/local/bin/
```

## Git Configuration

```bash
git config --global credential.helper github-app
git config --global gpg.format ssh
git config --global gpg.ssh.program git-ssh-sign
git config --global commit.gpgsign true
```

With these settings, `git push`, `git pull`, and `git commit -S` automatically use the auth service for credentials and signing.

## GitHub CLI Wrapper

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
