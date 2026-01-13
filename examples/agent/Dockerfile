# Example Agent Dockerfile
# Shows how to use the github-auth-helpers image in an agent container
#
# Build the helpers image first:
#   docker build -f Dockerfile.helpers -t github-auth-helpers .
#
# Then build this example:
#   docker build -f examples/agent/Dockerfile -t example-agent ./examples/agent

# Get the helper binaries from the helpers image
FROM github-auth-helpers:latest AS helpers

# Your actual agent base image
FROM python:3.12-slim

# Install git and gh CLI
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    ca-certificates \
    curl \
    && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update && apt-get install -y gh \
    && rm -rf /var/lib/apt/lists/*

# Copy helper binaries from helpers image
COPY --from=helpers /usr/local/bin/git-credential-github-app /usr/local/bin/
COPY --from=helpers /usr/local/bin/git-ssh-sign /usr/local/bin/
COPY --from=helpers /usr/local/bin/gh-github-app /usr/local/bin/

# Configure git to use the helpers
RUN git config --system credential.helper github-app && \
    git config --system gpg.format ssh && \
    git config --system gpg.ssh.program git-ssh-sign && \
    git config --system commit.gpgsign true && \
    git config --system user.signingkey "(managed by auth service)" && \
    # Replace gh with wrapper (wrapper finds real gh in /usr/bin/gh)
    mv /usr/bin/gh /usr/bin/gh-real && \
    ln -s /usr/local/bin/gh-github-app /usr/bin/gh

# Create non-root user
RUN useradd -m -s /bin/bash agent
USER agent
WORKDIR /home/agent

# Environment variables (these should be set at runtime via docker-compose or -e flags)
# ENV GITHUB_AUTH_SERVICE=http://github-auth-service:8080
# ENV AGENT_NAME=my-agent
# ENV AGENT_TOKEN=<token>

# Your agent entrypoint
# CMD ["python", "agent.py"]
CMD ["bash"]
