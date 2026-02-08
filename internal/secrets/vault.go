package secrets

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	vault "github.com/hashicorp/vault/api"
)

// VaultBackend implements SecretsBackend using HashiCorp Vault KV v2.
type VaultBackend struct {
	client    *vault.Client
	mountPath string // KV v2 engine mount path (e.g., "secret")
}

// Compile-time interface check.
var _ SecretsBackend = (*VaultBackend)(nil)

// NewVaultBackend creates a new HashiCorp Vault secrets backend.
// addr is the Vault server address (also read from VAULT_ADDR env).
// mountPath is the KV v2 mount point (default: "secret").
func NewVaultBackend(addr, mountPath string) (*VaultBackend, error) {
	config := vault.DefaultConfig()
	if addr != "" {
		config.Address = addr
	}

	client, err := vault.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Vault client: %w", err)
	}

	// Read token from VAULT_TOKEN (automatic) or VAULT_TOKEN_FILE
	if client.Token() == "" {
		tokenFile := os.Getenv("VAULT_TOKEN_FILE")
		if tokenFile != "" {
			data, err := os.ReadFile(tokenFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read VAULT_TOKEN_FILE: %w", err)
			}
			client.SetToken(string(bytes.TrimSpace(data)))
		}
	}

	if client.Token() == "" {
		return nil, fmt.Errorf("VAULT_TOKEN or VAULT_TOKEN_FILE is required")
	}

	if mountPath == "" {
		mountPath = "secret"
	}

	// Verify connectivity
	_, err = client.Auth().Token().LookupSelf()
	if err != nil {
		return nil, fmt.Errorf("failed to verify Vault token: %w", err)
	}

	slog.Info("connected to HashiCorp Vault", "addr", config.Address, "mount", mountPath)
	return &VaultBackend{client: client, mountPath: mountPath}, nil
}

// GetItems retrieves all agent items from the Vault KV v2 engine.
// The scope parameter is the base path under the mount (e.g., "agents").
func (v *VaultBackend) GetItems(ctx context.Context, scope string) ([]*Item, error) {
	// KV v2 list endpoint: <mount>/metadata/<path>
	listPath := fmt.Sprintf("%s/metadata/%s", v.mountPath, scope)

	secret, err := v.client.Logical().ListWithContext(ctx, listPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets at %s: %w", listPath, err)
	}

	if secret == nil || secret.Data == nil {
		return []*Item{}, nil
	}

	keys, ok := secret.Data["keys"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format from Vault list")
	}

	var items []*Item
	for _, k := range keys {
		key, ok := k.(string)
		if !ok {
			continue
		}
		// Skip directory entries (ending with /)
		if strings.HasSuffix(key, "/") {
			continue
		}

		// KV v2 read endpoint: <mount>/data/<path>/<key>
		readPath := fmt.Sprintf("%s/data/%s/%s", v.mountPath, scope, key)
		secret, err := v.client.Logical().ReadWithContext(ctx, readPath)
		if err != nil {
			slog.Warn("failed to read secret", "path", readPath, "error", err)
			continue
		}
		if secret == nil || secret.Data == nil {
			continue
		}

		// KV v2 wraps data in a "data" key
		data, ok := secret.Data["data"].(map[string]interface{})
		if !ok {
			slog.Warn("unexpected secret format", "path", readPath)
			continue
		}

		item := &Item{
			ID:     key,
			Name:   key,
			Fields: make(map[string]string),
		}

		for fieldKey, fieldVal := range data {
			if str, ok := fieldVal.(string); ok {
				item.Fields[fieldKey] = str
			}
		}

		items = append(items, item)
	}

	return items, nil
}

// GetAttachment is not supported by HashiCorp Vault.
// Private keys should be stored as field values instead.
func (v *VaultBackend) GetAttachment(ctx context.Context, itemID, attachmentName string) ([]byte, error) {
	return nil, ErrAttachmentsNotSupported
}

// Close is a no-op for Vault (tokens are typically long-lived or managed externally).
func (v *VaultBackend) Close(ctx context.Context) error {
	slog.Info("Vault backend closed")
	return nil
}

// Name returns the backend name for logging.
func (v *VaultBackend) Name() string {
	return "vault"
}
