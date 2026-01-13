#!/bin/bash
# seed-vault.sh - Seeds Vaultwarden with test GitHub App credentials
#
# This script:
# 1. Creates a test user and organization in Vaultwarden
# 2. Creates a collection for agents
# 3. Creates agent items with GitHub App credentials
# 4. Generates agent tokens (signed with the private key)
# 5. Exports session, collection ID, and agent tokens for the auth service
#
# Required environment variables:
#   VAULTWARDEN_URL
#   TEST_VAULT_EMAIL
#   TEST_VAULT_PASSWORD
#   TEST_GITHUB_APP_ID
#   TEST_GITHUB_INSTALLATION_ID
#   TEST_GITHUB_PRIVATE_KEY_B64
#   TEST_AGENT_NAME
#   TEST_AGENT_EMAIL

set -e

echo "=== Vault Seeder Starting ==="

# Configuration
BW_CLI="bw"
OUTPUT_DIR="${OUTPUT_DIR:-/output}"
MAX_RETRIES=30
RETRY_INTERVAL=2

# Validate required environment variables
required_vars=(
    "VAULTWARDEN_URL"
    "TEST_VAULT_EMAIL"
    "TEST_VAULT_PASSWORD"
    "TEST_GITHUB_APP_ID"
    "TEST_GITHUB_INSTALLATION_ID"
    "TEST_GITHUB_PRIVATE_KEY_B64"
    "TEST_AGENT_NAME"
)

for var in "${required_vars[@]}"; do
    if [ -z "${!var}" ]; then
        echo "ERROR: Required environment variable $var is not set"
        exit 1
    fi
done

# Wait for Vaultwarden to be ready
echo "Waiting for Vaultwarden at ${VAULTWARDEN_URL}..."
for i in $(seq 1 $MAX_RETRIES); do
    if curl -sf "${VAULTWARDEN_URL}/alive" > /dev/null 2>&1; then
        echo "Vaultwarden is ready!"
        break
    fi
    if [ $i -eq $MAX_RETRIES ]; then
        echo "ERROR: Vaultwarden failed to become ready"
        exit 1
    fi
    echo "  Attempt $i/$MAX_RETRIES - waiting..."
    sleep $RETRY_INTERVAL
done

# Configure Bitwarden CLI
echo "Configuring Bitwarden CLI..."
$BW_CLI config server "${VAULTWARDEN_URL}"

# Logout any existing session
$BW_CLI logout 2>/dev/null || true

# Try to login (user might already exist)
echo "Logging in to Bitwarden..."
if ! BW_SESSION=$($BW_CLI login "${TEST_VAULT_EMAIL}" "${TEST_VAULT_PASSWORD}" --raw 2>/dev/null); then
    echo "Login failed, creating new account..."

    # Try with unlock if already logged in
    BW_SESSION=$($BW_CLI unlock "${TEST_VAULT_PASSWORD}" --raw 2>/dev/null) || {
        echo "Creating account via Node.js registration helper..."
        # Use our Node.js script for proper password hashing (PBKDF2-SHA256)
        node /usr/local/bin/register-user.js "${VAULTWARDEN_URL}" "${TEST_VAULT_EMAIL}" "${TEST_VAULT_PASSWORD}" "Test User"

        sleep 2
        echo "Logging in after registration..."
        BW_SESSION=$($BW_CLI login "${TEST_VAULT_EMAIL}" "${TEST_VAULT_PASSWORD}" --raw)
    }
fi

export BW_SESSION
echo "Successfully authenticated!"

# Sync vault
echo "Syncing vault..."
$BW_CLI sync

# Create organization and collection
echo "Creating organization..."
ORG_NAME="test-agents-org"

# Check if org exists
ORG_ID=$($BW_CLI list organizations 2>/dev/null | jq -r ".[] | select(.name==\"${ORG_NAME}\") | .id" || echo "")

if [ -z "$ORG_ID" ]; then
    # Create organization via template
    ORG_JSON=$(cat <<EOF
{
    "name": "${ORG_NAME}",
    "billingEmail": "${TEST_VAULT_EMAIL}"
}
EOF
)
    # Note: bw CLI org creation is limited, using direct approach
    ORG_ID=$($BW_CLI create organization "$(echo "$ORG_JSON" | base64)" 2>/dev/null | jq -r '.id' || echo "")
fi

# If org creation failed, work without it (personal vault)
if [ -z "$ORG_ID" ]; then
    echo "Organization creation not supported, using personal vault..."
    COLLECTION_ID=""
else
    echo "Organization ID: $ORG_ID"

    # Create collection
    COLLECTION_NAME="github-app-agents"
    echo "Creating collection: ${COLLECTION_NAME}..."

    COLLECTION_ID=$($BW_CLI list collections --organizationid "$ORG_ID" 2>/dev/null | jq -r ".[] | select(.name==\"${COLLECTION_NAME}\") | .id" || echo "")

    if [ -z "$COLLECTION_ID" ]; then
        COLLECTION_JSON=$(cat <<EOF
{
    "organizationId": "${ORG_ID}",
    "name": "${COLLECTION_NAME}",
    "externalId": null
}
EOF
)
        COLLECTION_ID=$($BW_CLI create collection "$(echo "$COLLECTION_JSON" | base64)" --organizationid "$ORG_ID" 2>/dev/null | jq -r '.id' || echo "")
    fi
fi

echo "Collection ID: ${COLLECTION_ID:-none}"

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

# Read the private key content for storage
PRIVATE_KEY_DECODED=$(cat "${PRIVATE_KEY_FILE}")

# Generate agent token (sign agent name with private key)
echo "Generating agent token..."
AGENT_TOKEN=$(echo -n "${TEST_AGENT_NAME}" | openssl dgst -sha256 -sign "${PRIVATE_KEY_FILE}" | base64 | tr -d '\n')

# Create agent item
ITEM_NAME="${TEST_AGENT_NAME}"
echo "Creating Bitwarden item: ${ITEM_NAME}..."

# Check if item exists
EXISTING_ITEM=$($BW_CLI list items --search "${ITEM_NAME}" 2>/dev/null | jq -r ".[0].id" || echo "")

if [ -n "$EXISTING_ITEM" ] && [ "$EXISTING_ITEM" != "null" ]; then
    echo "Item exists, deleting..."
    $BW_CLI delete item "$EXISTING_ITEM" --permanent 2>/dev/null || true
    $BW_CLI sync
fi

# Escape the private key for JSON (replace newlines with \n)
PRIVATE_KEY_ESCAPED=$(echo "${PRIVATE_KEY_DECODED}" | sed ':a;N;$!ba;s/\n/\\n/g')

# Create item JSON with private key in a hidden field
ITEM_JSON=$(cat <<EOF
{
    "type": 2,
    "name": "${ITEM_NAME}",
    "notes": "Test GitHub App agent for e2e testing",
    "secureNote": {
        "type": 0
    },
    "fields": [
        {"name": "app_id", "value": "${TEST_GITHUB_APP_ID}", "type": 0},
        {"name": "installation_id", "value": "${TEST_GITHUB_INSTALLATION_ID}", "type": 0},
        {"name": "agent_token", "value": "${AGENT_TOKEN}", "type": 1},
        {"name": "private_key", "value": "${PRIVATE_KEY_ESCAPED}", "type": 1},
        {"name": "identity_name", "value": "${TEST_AGENT_NAME}", "type": 0},
        {"name": "identity_email", "value": "${TEST_AGENT_EMAIL:-${TEST_AGENT_NAME}@example.com}", "type": 0}
    ]
}
EOF
)

# Add collection if available
if [ -n "$COLLECTION_ID" ]; then
    ITEM_JSON=$(echo "$ITEM_JSON" | jq --arg cid "$COLLECTION_ID" '. + {collectionIds: [$cid]}')
fi

# Create the item
echo "Creating item..."
CREATED_ITEM=$($BW_CLI create item "$(echo "$ITEM_JSON" | base64)")
ITEM_ID=$(echo "$CREATED_ITEM" | jq -r '.id')
echo "Created item with ID: ${ITEM_ID}"

# Final sync
$BW_CLI sync

# Export outputs
echo "Exporting outputs..."
mkdir -p "${OUTPUT_DIR}"

# Session token (readable by other containers)
echo "${BW_SESSION}" > "${OUTPUT_DIR}/session.txt"
chmod 644 "${OUTPUT_DIR}/session.txt"

# Collection ID (or empty if using personal vault)
if [ -n "$COLLECTION_ID" ]; then
    echo "${COLLECTION_ID}" > "${OUTPUT_DIR}/collection_id.txt"
else
    # For personal vault, we need to handle differently
    # The auth service will need to list all items
    echo "PERSONAL_VAULT" > "${OUTPUT_DIR}/collection_id.txt"
fi
chmod 644 "${OUTPUT_DIR}/collection_id.txt"

# Agent token for test runner (readable by other containers)
echo "${AGENT_TOKEN}" > "${OUTPUT_DIR}/agent_token.txt"
chmod 644 "${OUTPUT_DIR}/agent_token.txt"

# Agent name
echo "${TEST_AGENT_NAME}" > "${OUTPUT_DIR}/agent_name.txt"
chmod 644 "${OUTPUT_DIR}/agent_name.txt"

echo ""
echo "=== Vault Seeder Complete ==="
echo "  Item created: ${ITEM_NAME}"
echo "  Item ID: ${ITEM_ID}"
echo "  Collection ID: ${COLLECTION_ID:-personal vault}"
echo "  Session: ${OUTPUT_DIR}/session.txt"
echo "  Agent token: ${OUTPUT_DIR}/agent_token.txt"
echo ""

# Cleanup
rm -f "${PRIVATE_KEY_FILE}"

# Keep seeder running briefly to allow dependent services to start
# (docker-compose --exit-code-from stops all containers when any exit)
echo "Waiting for dependent services to initialize..."
sleep 30
