package secrets

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// AgentSecrets holds all secrets for a single agent, loaded at startup.
type AgentSecrets struct {
	Name           string          // Agent name (from Bitwarden item name)
	AppID          string          // GitHub App ID
	InstallationID string          // GitHub App Installation ID
	PrivateKey     *rsa.PrivateKey // Parsed RSA private key
	PrivateKeyPEM  []byte          // Raw PEM for SSH signing
	AgentToken     string          // Pre-signed token for authentication
	Identity       Identity        // Git identity (name, email)
}

// Identity holds git commit identity information.
type Identity struct {
	Name  string
	Email string
}

// Store holds all agent secrets in memory.
// Secrets are loaded once at startup and the Bitwarden session is closed.
type Store struct {
	agents map[string]*AgentSecrets
	mu     sync.RWMutex
}

// NewStore creates an empty secrets store.
func NewStore() *Store {
	return &Store{
		agents: make(map[string]*AgentSecrets),
	}
}

// LoadFromBitwarden fetches all agent secrets from a Bitwarden collection,
// stores them in memory, and logs out the session.
func (s *Store) LoadFromBitwarden(ctx context.Context, backend *BitwardenBackend, collectionID string) error {
	slog.Info("loading secrets from Bitwarden collection", "collection_id", collectionID)

	var items []*Item
	var err error

	// Handle personal vault (no collection) vs organization collection
	if collectionID == "PERSONAL_VAULT" || collectionID == "" {
		slog.Info("using personal vault - listing all items")
		items, err = backend.ListAllItems(ctx)
	} else {
		items, err = backend.GetCollectionItems(ctx, collectionID)
	}
	if err != nil {
		return fmt.Errorf("failed to get items: %w", err)
	}

	slog.Info("found items in collection", "count", len(items))

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, item := range items {
		agent, err := s.parseAgentItem(ctx, backend, item)
		if err != nil {
			slog.Warn("failed to parse agent item",
				"item_name", item.Name,
				"error", err,
			)
			continue
		}

		s.agents[agent.Name] = agent
		slog.Info("loaded agent secrets",
			"agent", agent.Name,
			"app_id", agent.AppID,
			"identity", agent.Identity.Email,
		)
	}

	// Logout the Bitwarden session
	if err := backend.Logout(ctx); err != nil {
		slog.Warn("failed to logout Bitwarden session", "error", err)
		// Don't fail - secrets are already loaded
	} else {
		slog.Info("Bitwarden session closed")
	}

	if len(s.agents) == 0 {
		return fmt.Errorf("no agents loaded from collection")
	}

	return nil
}

// parseAgentItem converts a Bitwarden item into AgentSecrets.
func (s *Store) parseAgentItem(ctx context.Context, backend *BitwardenBackend, item *Item) (*AgentSecrets, error) {
	agent := &AgentSecrets{
		Name: item.Name,
	}

	// Extract required fields
	var ok bool
	if agent.AppID, ok = item.Fields["app_id"]; !ok {
		return nil, fmt.Errorf("missing app_id field")
	}
	if agent.InstallationID, ok = item.Fields["installation_id"]; !ok {
		return nil, fmt.Errorf("missing installation_id field")
	}
	if agent.AgentToken, ok = item.Fields["agent_token"]; !ok {
		return nil, fmt.Errorf("missing agent_token field")
	}

	// Extract identity (optional, with defaults)
	agent.Identity.Name = item.Fields["identity_name"]
	if agent.Identity.Name == "" {
		agent.Identity.Name = item.Name
	}
	agent.Identity.Email = item.Fields["identity_email"]
	if agent.Identity.Email == "" {
		agent.Identity.Email = fmt.Sprintf("%s@users.noreply.github.com", item.Name)
	}

	// Get private key - try field first (for testing), then attachment
	var privateKeyPEM []byte
	var pkErr error
	if privateKeyStr, ok := item.Fields["private_key"]; ok && privateKeyStr != "" {
		// Private key stored in field - unescape newlines
		privateKeyPEM = []byte(strings.ReplaceAll(privateKeyStr, "\\n", "\n"))
	} else {
		// Try to get from attachment
		privateKeyPEM, pkErr = backend.GetAttachment(ctx, item.ID, "private-key.pem")
		if pkErr != nil {
			return nil, fmt.Errorf("failed to get private key: %w", pkErr)
		}
	}
	agent.PrivateKeyPEM = privateKeyPEM

	// Parse the private key
	agent.PrivateKey, pkErr = parseRSAPrivateKey(privateKeyPEM)
	if pkErr != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", pkErr)
	}

	return agent, nil
}

// GetAgent retrieves an agent's secrets by name.
func (s *Store) GetAgent(name string) (*AgentSecrets, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agent, ok := s.agents[name]
	return agent, ok
}

// VerifyAgentToken verifies that the provided token was signed with the agent's private key.
func (s *Store) VerifyAgentToken(agentName, token string) bool {
	s.mu.RLock()
	agent, ok := s.agents[agentName]
	s.mu.RUnlock()

	if !ok {
		return false
	}

	// The token is: base64(sign(sha256(agentName), privateKey))
	// We verify by checking if the signature matches
	return verifySignature(agent.PrivateKey, agentName, token)
}

// ListAgents returns the names of all loaded agents.
func (s *Store) ListAgents() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.agents))
	for name := range s.agents {
		names = append(names, name)
	}
	return names
}

// Count returns the number of loaded agents.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.agents)
}

// parseRSAPrivateKey parses a PEM-encoded RSA private key.
func parseRSAPrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Try PKCS#1 format first (RSA PRIVATE KEY)
	if block.Type == "RSA PRIVATE KEY" {
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}

	// Try PKCS#8 format (PRIVATE KEY)
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not an RSA private key")
	}

	return rsaKey, nil
}

// verifySignature verifies that the token is a valid signature of the data.
func verifySignature(publicKey *rsa.PrivateKey, data, token string) bool {
	// Decode the base64 token
	signature, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return false
	}

	// Hash the data
	hash := sha256.Sum256([]byte(data))

	// Verify the signature using the public key portion
	err = rsa.VerifyPKCS1v15(&publicKey.PublicKey, crypto.SHA256, hash[:], signature)
	return err == nil
}

// GenerateAgentToken creates a signed token for an agent.
// This is a utility function for setting up new agents.
func GenerateAgentToken(privateKey *rsa.PrivateKey, agentName string) (string, error) {
	hash := sha256.Sum256([]byte(agentName))
	signature, err := rsa.SignPKCS1v15(nil, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign: %w", err)
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}
