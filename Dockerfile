# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy source code
COPY . .

# Download dependencies (go mod tidy generates go.sum if needed)
RUN go mod tidy

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /github-auth-service ./cmd/server

# Runtime stage
FROM node:20-alpine

# Install runtime dependencies and build tools for Bitwarden CLI (argon2 needs compilation)
RUN apk add --no-cache \
    ca-certificates \
    curl \
    python3 \
    make \
    g++ \
    && npm install -g @bitwarden/cli@2024.2.0 \
    && apk del python3 make g++ \
    && rm -rf /var/cache/apk/* /root/.npm /tmp/*

WORKDIR /app

# Copy binary from builder
COPY --from=builder /github-auth-service /usr/local/bin/github-auth-service

# Set ownership (node user is UID 1000 in node:alpine images)
RUN chown node:node /usr/local/bin/github-auth-service

# Switch to non-root user
USER node

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Run the service
CMD ["github-auth-service"]
