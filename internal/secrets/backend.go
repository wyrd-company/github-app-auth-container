package secrets

import (
	"context"
	"errors"
)

// SecretsBackend abstracts a secrets storage provider.
// Implementations include Bitwarden/Vaultwarden and HashiCorp Vault.
type SecretsBackend interface {
	// GetItems retrieves all agent items from the backend.
	// The scope parameter is backend-specific:
	//   - Bitwarden: collectionID (or "PERSONAL_VAULT" / empty for all items)
	//   - Vault: base path in KV engine (e.g., "agents")
	GetItems(ctx context.Context, scope string) ([]*Item, error)

	// GetAttachment retrieves attachment data by item ID and filename.
	// Backends that don't support attachments return ErrAttachmentsNotSupported.
	GetAttachment(ctx context.Context, itemID, attachmentName string) ([]byte, error)

	// Close performs cleanup (lock session, revoke token, etc.).
	// Called after all secrets have been loaded.
	Close(ctx context.Context) error

	// Name returns a human-readable backend name for logging.
	Name() string
}

// ErrAttachmentsNotSupported is returned by backends that don't support attachments.
var ErrAttachmentsNotSupported = errors.New("attachments not supported by this backend")

// Item represents a secrets item with fields and attachments.
type Item struct {
	ID          string
	Name        string
	Fields      map[string]string
	Attachments []AttachmentInfo
	Notes       string
}

// AttachmentInfo holds metadata about an attachment.
type AttachmentInfo struct {
	ID       string
	FileName string
	Size     int64
}
