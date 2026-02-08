.PHONY: build test test-e2e test-e2e-vault clean help deps

# Default target
help:
	@echo "GitHub App Auth Container"
	@echo ""
	@echo "Usage:"
	@echo "  make deps             - Download Go dependencies and generate go.sum"
	@echo "  make build            - Build the Docker image"
	@echo "  make test             - Run unit tests"
	@echo "  make test-e2e         - Run e2e tests with Vaultwarden (Bitwarden backend)"
	@echo "  make test-e2e-vault   - Run e2e tests with HashiCorp Vault backend"
	@echo "  make clean            - Clean up containers and volumes"
	@echo ""
	@echo "E2E Test Setup:"
	@echo "  1. Copy .env.example to .env"
	@echo "  2. Fill in your test GitHub App credentials"
	@echo "  3. Run: make test-e2e  (or make test-e2e-vault)"

# Download dependencies
deps:
	go mod download
	go mod tidy

# Build the main Docker image
build:
	docker build -t github-app-auth-container:latest .

# Run unit tests (requires Go installed)
test:
	go test -v -short ./...

# Run e2e tests with Vaultwarden
test-e2e: check-env
	@echo "Building test containers..."
	docker-compose -f docker-compose.test.yml build

	@echo "Starting Vaultwarden..."
	docker-compose -f docker-compose.test.yml up -d vaultwarden

	@echo "Waiting for Vaultwarden to be ready..."
	@timeout 60 bash -c 'until curl -sf http://localhost:80/alive 2>/dev/null; do sleep 2; done' || \
		(docker-compose -f docker-compose.test.yml logs vaultwarden && exit 1)

	@echo "Seeding vault with test credentials..."
	docker-compose -f docker-compose.test.yml up vault-seeder

	@echo "Starting auth service..."
	docker-compose -f docker-compose.test.yml up -d github-auth-service

	@echo "Waiting for auth service to be ready..."
	@timeout 60 bash -c 'until curl -sf http://localhost:8080/health 2>/dev/null; do sleep 2; done' || \
		(docker-compose -f docker-compose.test.yml logs github-auth-service && exit 1)

	@echo "Running e2e tests..."
	docker-compose -f docker-compose.test.yml up test-runner

	@echo "Tests completed. Cleaning up..."
	docker-compose -f docker-compose.test.yml down -v

# Run e2e tests with HashiCorp Vault
test-e2e-vault: check-env
	@echo "Building test containers (Vault backend)..."
	docker-compose -f docker-compose.test-vault.yml build

	@echo "Starting Vault server..."
	docker-compose -f docker-compose.test-vault.yml up -d vault-server

	@echo "Waiting for Vault to be ready..."
	@timeout 30 bash -c 'until curl -sf http://localhost:8200/v1/sys/health 2>/dev/null; do sleep 1; done' || \
		(docker-compose -f docker-compose.test-vault.yml logs vault-server && exit 1)

	@echo "Seeding Vault with test credentials..."
	docker-compose -f docker-compose.test-vault.yml up vault-seeder

	@echo "Starting auth service..."
	docker-compose -f docker-compose.test-vault.yml up -d github-auth-service

	@echo "Waiting for auth service to be ready..."
	@timeout 60 bash -c 'until curl -sf http://localhost:8080/health 2>/dev/null; do sleep 2; done' || \
		(docker-compose -f docker-compose.test-vault.yml logs github-auth-service && exit 1)

	@echo "Running e2e tests..."
	docker-compose -f docker-compose.test-vault.yml up test-runner

	@echo "Tests completed. Cleaning up..."
	docker-compose -f docker-compose.test-vault.yml down -v

# Check that .env exists
check-env:
	@if [ ! -f .env ]; then \
		echo "Error: .env file not found. Copy .env.example to .env and fill in credentials."; \
		exit 1; \
	fi

# Clean up all test containers and volumes
clean:
	docker-compose -f docker-compose.test.yml down -v --remove-orphans 2>/dev/null || true
	docker-compose -f docker-compose.test-vault.yml down -v --remove-orphans 2>/dev/null || true
	docker-compose down -v --remove-orphans 2>/dev/null || true

# Run the service locally (requires .env with BW_SESSION)
run:
	docker-compose up --build

# Show logs from test containers
logs:
	docker-compose -f docker-compose.test.yml logs -f

# Quick health check
health:
	@curl -sf http://localhost:8080/health | jq . || echo "Service not running"
