#!/bin/bash
# run-e2e-tests.sh - Runs e2e tests with proper container orchestration
#
# Usage: ./scripts/run-e2e-tests.sh

set -e

echo "=== Starting E2E Test Stack ==="

# Clean up any previous run
docker-compose -f docker-compose.test.yml down -v 2>/dev/null || true

# Build and start all services
echo "Building and starting services..."
docker-compose -f docker-compose.test.yml up --build -d

# Wait for test-runner to complete
echo "Waiting for tests to complete..."
docker wait test-runner 2>/dev/null || {
    # test-runner might not have started yet, wait a bit
    sleep 5
    docker wait test-runner
}

# Get the exit code
EXIT_CODE=$(docker inspect test-runner --format='{{.State.ExitCode}}')

# Show logs from all containers
echo ""
echo "=== Test Runner Logs ==="
docker logs test-runner

echo ""
echo "=== Auth Service Logs ==="
docker logs test-github-auth-service 2>/dev/null || echo "(no logs)"

echo ""
echo "=== Vault Seeder Logs ==="
docker logs test-vault-seeder 2>/dev/null || echo "(no logs)"

# Clean up
echo ""
echo "Cleaning up..."
docker-compose -f docker-compose.test.yml down -v

# Exit with test-runner's exit code
echo ""
if [ "$EXIT_CODE" = "0" ]; then
    echo "=== Tests PASSED ==="
else
    echo "=== Tests FAILED (exit code: $EXIT_CODE) ==="
fi

exit $EXIT_CODE
