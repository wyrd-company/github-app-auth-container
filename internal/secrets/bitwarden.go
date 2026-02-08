package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// BitwardenBackend implements secrets fetching using the Bitwarden CLI.
type BitwardenBackend struct {
	session   string
	serverURL string
}

// bwItem represents the JSON structure returned by `bw get item`.
type bwItem struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Notes       string         `json:"notes"`
	Fields      []bwField      `json:"fields"`
	Attachments []bwAttachment `json:"attachments"`
}

type bwField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  int    `json:"type"`
}

type bwAttachment struct {
	ID       string `json:"id"`
	FileName string `json:"fileName"`
	Size     int64  `json:"size"`
}

// Compile-time interface check.
var _ SecretsBackend = (*BitwardenBackend)(nil)

// NewBitwardenBackend creates a new Bitwarden secrets backend.
func NewBitwardenBackend(serverURL string) (*BitwardenBackend, error) {
	session := os.Getenv("BW_SESSION")

	// Also check for session file (used in testing)
	if session == "" {
		sessionFile := os.Getenv("BW_SESSION_FILE")
		if sessionFile != "" {
			data, err := os.ReadFile(sessionFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read BW_SESSION_FILE: %w", err)
			}
			session = string(bytes.TrimSpace(data))
		}
	}

	if session == "" {
		return nil, fmt.Errorf("BW_SESSION or BW_SESSION_FILE environment variable is required")
	}

	backend := &BitwardenBackend{
		session:   session,
		serverURL: serverURL,
	}

	// Configure server URL if provided (for self-hosted Bitwarden/Vaultwarden)
	// This MUST be done before any other CLI operations, including sync
	if serverURL != "" {
		slog.Info("configuring bitwarden server", "url", serverURL)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := backend.runBWNoSession(ctx, nil, "config", "server", serverURL); err != nil {
			return nil, fmt.Errorf("failed to configure bitwarden server: %w", err)
		}
	}

	// Sync to get latest data
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := backend.Sync(ctx); err != nil {
		return nil, fmt.Errorf("failed to sync: %w", err)
	}

	return backend, nil
}

// runBW executes a Bitwarden CLI command and returns the output.
func (b *BitwardenBackend) runBW(ctx context.Context, output *bytes.Buffer, args ...string) error {
	cmd := exec.CommandContext(ctx, "bw", args...)
	cmd.Env = append(os.Environ(), "BW_SESSION="+b.session)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bw command failed: %w, stderr: %s", err, stderr.String())
	}

	if output != nil {
		output.Write(stdout.Bytes())
	}

	return nil
}

// runBWNoSession executes a Bitwarden CLI command without a session (for config commands).
func (b *BitwardenBackend) runBWNoSession(ctx context.Context, output *bytes.Buffer, args ...string) error {
	cmd := exec.CommandContext(ctx, "bw", args...)
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bw command failed: %w, stderr: %s", err, stderr.String())
	}

	if output != nil {
		output.Write(stdout.Bytes())
	}

	return nil
}

// Sync synchronizes the local Bitwarden cache with the server.
func (b *BitwardenBackend) Sync(ctx context.Context) error {
	if err := b.runBW(ctx, nil, "sync"); err != nil {
		return fmt.Errorf("bitwarden sync failed: %w", err)
	}
	slog.Debug("bitwarden sync completed")
	return nil
}

// Logout closes the Bitwarden session.
func (b *BitwardenBackend) Logout(ctx context.Context) error {
	// Lock the vault instead of full logout (logout requires re-auth)
	if err := b.runBW(ctx, nil, "lock"); err != nil {
		return fmt.Errorf("bitwarden lock failed: %w", err)
	}
	b.session = "" // Clear session in memory
	return nil
}

// ListAllItems retrieves all items in the vault (personal vault).
func (b *BitwardenBackend) ListAllItems(ctx context.Context) ([]*Item, error) {
	var output bytes.Buffer
	if err := b.runBW(ctx, &output, "list", "items"); err != nil {
		return nil, fmt.Errorf("failed to list items: %w", err)
	}

	var bwItems []bwItem
	if err := json.Unmarshal(output.Bytes(), &bwItems); err != nil {
		return nil, fmt.Errorf("failed to parse items: %w", err)
	}

	items := make([]*Item, 0, len(bwItems))
	for _, bwItem := range bwItems {
		item := &Item{
			ID:     bwItem.ID,
			Name:   bwItem.Name,
			Notes:  bwItem.Notes,
			Fields: make(map[string]string),
		}

		for _, field := range bwItem.Fields {
			item.Fields[field.Name] = field.Value
		}

		for _, att := range bwItem.Attachments {
			item.Attachments = append(item.Attachments, AttachmentInfo{
				ID:       att.ID,
				FileName: att.FileName,
				Size:     att.Size,
			})
		}

		items = append(items, item)
	}

	return items, nil
}

// GetCollectionItems retrieves all items in a collection.
func (b *BitwardenBackend) GetCollectionItems(ctx context.Context, collectionID string) ([]*Item, error) {
	var output bytes.Buffer
	if err := b.runBW(ctx, &output, "list", "items", "--collectionid", collectionID); err != nil {
		return nil, fmt.Errorf("failed to list collection items: %w", err)
	}

	var bwItems []bwItem
	if err := json.Unmarshal(output.Bytes(), &bwItems); err != nil {
		return nil, fmt.Errorf("failed to parse items: %w", err)
	}

	items := make([]*Item, 0, len(bwItems))
	for _, bwItem := range bwItems {
		item := &Item{
			ID:     bwItem.ID,
			Name:   bwItem.Name,
			Notes:  bwItem.Notes,
			Fields: make(map[string]string),
		}

		for _, field := range bwItem.Fields {
			item.Fields[field.Name] = field.Value
		}

		for _, att := range bwItem.Attachments {
			item.Attachments = append(item.Attachments, AttachmentInfo{
				ID:       att.ID,
				FileName: att.FileName,
				Size:     att.Size,
			})
		}

		items = append(items, item)
	}

	return items, nil
}

// GetItem retrieves an item by name or ID.
func (b *BitwardenBackend) GetItem(ctx context.Context, itemID string) (*Item, error) {
	var output bytes.Buffer
	if err := b.runBW(ctx, &output, "get", "item", itemID); err != nil {
		return nil, fmt.Errorf("failed to get item: %w", err)
	}

	var bwItem bwItem
	if err := json.Unmarshal(output.Bytes(), &bwItem); err != nil {
		return nil, fmt.Errorf("failed to parse item: %w", err)
	}

	item := &Item{
		ID:     bwItem.ID,
		Name:   bwItem.Name,
		Notes:  bwItem.Notes,
		Fields: make(map[string]string),
	}

	for _, field := range bwItem.Fields {
		item.Fields[field.Name] = field.Value
	}

	for _, att := range bwItem.Attachments {
		item.Attachments = append(item.Attachments, AttachmentInfo{
			ID:       att.ID,
			FileName: att.FileName,
			Size:     att.Size,
		})
	}

	return item, nil
}

// GetAttachment retrieves attachment contents from a Bitwarden item.
func (b *BitwardenBackend) GetAttachment(ctx context.Context, itemID, attachmentName string) ([]byte, error) {
	// Create a temporary directory for the attachment
	tmpDir, err := os.MkdirTemp("", "bw-attachment-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPath := filepath.Join(tmpDir, attachmentName)

	// Download the attachment
	if err := b.runBW(ctx, nil, "get", "attachment", attachmentName, "--itemid", itemID, "--output", outputPath); err != nil {
		return nil, fmt.Errorf("failed to get attachment: %w", err)
	}

	// Read the file
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read attachment file: %w", err)
	}

	return data, nil
}

// GetItems retrieves all agent items, delegating to ListAllItems or GetCollectionItems
// based on the scope parameter (Bitwarden collection ID).
func (b *BitwardenBackend) GetItems(ctx context.Context, scope string) ([]*Item, error) {
	if scope == "PERSONAL_VAULT" || scope == "" {
		return b.ListAllItems(ctx)
	}
	return b.GetCollectionItems(ctx, scope)
}

// Close locks the Bitwarden vault and clears the session.
func (b *BitwardenBackend) Close(ctx context.Context) error {
	return b.Logout(ctx)
}

// Name returns the backend name for logging.
func (b *BitwardenBackend) Name() string {
	return "bitwarden"
}
