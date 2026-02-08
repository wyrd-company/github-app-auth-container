#!/bin/bash
# seed-vault-hcv.sh - Seeds HashiCorp Vault with test GitHub App credentials
#
# This script:
# 1. Waits for Vault to be ready
# 2. Decodes or generates a GitHub App private key
# 3. Generates an agent token (signed with the private key)
# 4. Writes agent secrets to Vault KV v2
# 5. Exports agent token for the test runner
#
# Required environment variables:
#   VAULT_ADDR
#   VAULT_TOKEN
#   TEST_GITHUB_APP_ID
#   TEST_GITHUB_INSTALLATION_ID
#   TEST_GITHUB_PRIVATE_KEY_B64
#   TEST_AGENT_NAME
#   TEST_AGENT_EMAIL

set -e

echo "=== HashiCorp Vault Seeder Starting ==="

# Configuration
OUTPUT_DIR="${OUTPUT_DIR:-/output}"
MAX_RETRIES=30
RETRY_INTERVAL=2

# Validate required environment variables
required_vars=(
    "VAULT_ADDR"
    "VAULT_TOKEN"
    "TEST_AGENT_NAME"
)

for var in "${required_vars[@]}"; do
    if [ -z "${!var}" ]; then
        echo "ERROR: Required environment variable $var is not set"
        exit 1
    fi
done

# Apply defaults for optional GitHub App settings (allows CI without real secrets)
TEST_GITHUB_APP_ID="${TEST_GITHUB_APP_ID:-123456}"
TEST_GITHUB_INSTALLATION_ID="${TEST_GITHUB_INSTALLATION_ID:-78901234}"

# Wait for Vault to be ready
echo "Waiting for Vault at ${VAULT_ADDR}..."
for i in $(seq 1 $MAX_RETRIES); do
    if vault status > /dev/null 2>&1; then
        echo "Vault is ready!"
        break
    fi
    if [ $i -eq $MAX_RETRIES ]; then
        echo "ERROR: Vault failed to become ready"
        exit 1
    fi
    echo "  Attempt $i/$MAX_RETRIES - waiting..."
    sleep $RETRY_INTERVAL
done

# Check if agent secret already exists (idempotent re-runs)
EXISTING_TOKEN=$(vault kv get -field=agent_token "secret/agents/${TEST_AGENT_NAME}" 2>/dev/null || echo "")
if [ -n "${EXISTING_TOKEN}" ]; then
    echo "Agent secret already exists in Vault, skipping seed."
    mkdir -p "${OUTPUT_DIR}"
    echo "${EXISTING_TOKEN}" > "${OUTPUT_DIR}/agent_token.txt"
    chmod 600 "${OUTPUT_DIR}/agent_token.txt"
    echo "${TEST_AGENT_NAME}" > "${OUTPUT_DIR}/agent_name.txt"
    chmod 644 "${OUTPUT_DIR}/agent_name.txt"
    echo ""
    echo "=== HashiCorp Vault Seeder Complete (existing data) ==="
    echo "  Agent: ${TEST_AGENT_NAME}"
    echo "  Agent token: ${OUTPUT_DIR}/agent_token.txt"
    echo ""
    exit 0
fi

# Decode or generate private key
echo "Processing private key..."
PRIVATE_KEY_FILE="/tmp/github-app-private-key.pem"

if [ -n "${TEST_GITHUB_PRIVATE_KEY_B64}" ] && [ "${TEST_GITHUB_PRIVATE_KEY_B64}" != "LS0tLS1CRUdJTi...base64..." ]; then
    echo "  Decoding provided private key..."
    echo "${TEST_GITHUB_PRIVATE_KEY_B64}" | base64 -d > "${PRIVATE_KEY_FILE}"
else
    echo "  No private key provided, generating test key..."
    openssl genrsa -out "${PRIVATE_KEY_FILE}" 2048 2>/dev/null
fi
chmod 600 "${PRIVATE_KEY_FILE}"

# Verify the key is valid
if ! openssl rsa -in "${PRIVATE_KEY_FILE}" -check -noout 2>/dev/null; then
    echo "ERROR: Invalid private key. Please check TEST_GITHUB_PRIVATE_KEY_B64."
    exit 1
fi
echo "  Private key is valid"

# Read the private key content
PRIVATE_KEY_DECODED=$(cat "${PRIVATE_KEY_FILE}")

# Generate agent token (sign agent name with private key)
echo "Generating agent token..."
AGENT_TOKEN=$(echo -n "${TEST_AGENT_NAME}" | openssl dgst -sha256 -sign "${PRIVATE_KEY_FILE}" | base64 | tr -d '\n')

# Escape the private key for storage (replace newlines with \n)
PRIVATE_KEY_ESCAPED=$(echo "${PRIVATE_KEY_DECODED}" | sed ':a;N;$!ba;s/\n/\\n/g')

# Write agent secrets to Vault KV v2
AGENT_PATH="secret/agents/${TEST_AGENT_NAME}"
echo "Writing agent secrets to Vault at ${AGENT_PATH}..."

vault kv put "secret/agents/${TEST_AGENT_NAME}" \
    app_id="${TEST_GITHUB_APP_ID}" \
    installation_id="${TEST_GITHUB_INSTALLATION_ID}" \
    agent_token="${AGENT_TOKEN}" \
    private_key="${PRIVATE_KEY_ESCAPED}" \
    identity_name="${TEST_AGENT_NAME}" \
    identity_email="${TEST_AGENT_EMAIL:-${TEST_AGENT_NAME}@example.com}"

echo "Agent secrets written successfully"

# Verify the secret was written
echo "Verifying secret..."
vault kv get "secret/agents/${TEST_AGENT_NAME}" > /dev/null
echo "Secret verified"

# Export outputs
echo "Exporting outputs..."
mkdir -p "${OUTPUT_DIR}"

echo "${AGENT_TOKEN}" > "${OUTPUT_DIR}/agent_token.txt"
chmod 600 "${OUTPUT_DIR}/agent_token.txt"

echo "${TEST_AGENT_NAME}" > "${OUTPUT_DIR}/agent_name.txt"
chmod 644 "${OUTPUT_DIR}/agent_name.txt"

echo ""
echo "=== HashiCorp Vault Seeder Complete ==="
echo "  Agent: ${TEST_AGENT_NAME}"
echo "  Path: ${AGENT_PATH}"
echo "  Agent token: ${OUTPUT_DIR}/agent_token.txt"
echo ""

# Cleanup
rm -f "${PRIVATE_KEY_FILE}"
