package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/boblangley/github-app-auth-container/internal/secrets"
	"github.com/golang-jwt/jwt/v5"
)

// Service handles GitHub App authentication operations.
type Service struct {
	apiURL string
	cache  *tokenCache
	client *http.Client
}

// NewService creates a new GitHub authentication service.
func NewService(apiURL string) *Service {
	if apiURL == "" {
		apiURL = "https://api.github.com"
	}
	return &Service{
		apiURL: apiURL,
		cache:  newTokenCache(5 * time.Minute),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// InstallationToken represents a GitHub App installation access token.
type InstallationToken struct {
	Token               string            `json:"token"`
	ExpiresAt           time.Time         `json:"expires_at"`
	Permissions         map[string]string `json:"permissions"`
	RepositorySelection string            `json:"repository_selection"`
}

// TokenRequest specifies options for token generation.
type TokenRequest struct {
	Repositories []string          `json:"repositories,omitempty"`
	Permissions  map[string]string `json:"permissions,omitempty"`
}

// GetInstallationToken retrieves an installation access token for a GitHub App.
func (s *Service) GetInstallationToken(ctx context.Context, agent *secrets.AgentSecrets, req *TokenRequest) (*InstallationToken, error) {
	// Check cache first
	cacheKey := s.cacheKey(agent.Name, req)
	if cached := s.cache.get(cacheKey); cached != nil {
		slog.Debug("returning cached token", "agent", agent.Name)
		return cached, nil
	}

	// Generate JWT
	jwtToken, err := s.generateJWT(agent.AppID, agent.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	// Request installation token from GitHub
	token, err := s.requestInstallationToken(ctx, jwtToken, agent.InstallationID, req)
	if err != nil {
		return nil, err
	}

	// Cache the token
	s.cache.set(cacheKey, token)

	slog.Debug("fetched new installation token",
		"agent", agent.Name,
		"expires_at", token.ExpiresAt,
	)

	return token, nil
}

// generateJWT creates a JWT for GitHub App authentication.
func (s *Service) generateJWT(appID string, privateKey interface{}) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		// Issued 60 seconds in the past to handle clock drift
		"iat": now.Add(-60 * time.Second).Unix(),
		// Expires in 9 minutes (GitHub max is 10 minutes)
		"exp": now.Add(9 * time.Minute).Unix(),
		// GitHub App ID
		"iss": appID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	return signedToken, nil
}

// requestInstallationToken makes the API call to GitHub to get an installation token.
func (s *Service) requestInstallationToken(ctx context.Context, jwtToken, installationID string, req *TokenRequest) (*InstallationToken, error) {
	url := fmt.Sprintf("%s/app/installations/%s/access_tokens", s.apiURL, installationID)

	var body io.Reader
	if req != nil && (len(req.Repositories) > 0 || len(req.Permissions) > 0) {
		jsonBody, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		body = bytes.NewReader(jsonBody)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("Authorization", "Bearer "+jwtToken)
	httpReq.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to request token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error: %d - %s", resp.StatusCode, string(respBody))
	}

	var tokenResp struct {
		Token               string            `json:"token"`
		ExpiresAt           string            `json:"expires_at"`
		Permissions         map[string]string `json:"permissions"`
		RepositorySelection string            `json:"repository_selection"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	expiresAt, err := time.Parse(time.RFC3339, tokenResp.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse expiry time: %w", err)
	}

	return &InstallationToken{
		Token:               tokenResp.Token,
		ExpiresAt:           expiresAt,
		Permissions:         tokenResp.Permissions,
		RepositorySelection: tokenResp.RepositorySelection,
	}, nil
}

// cacheKey generates a cache key for a token request.
func (s *Service) cacheKey(agentName string, req *TokenRequest) string {
	key := agentName
	if req != nil {
		if len(req.Repositories) > 0 {
			for _, r := range req.Repositories {
				key += ":" + r
			}
		}
		if len(req.Permissions) > 0 {
			for k, v := range req.Permissions {
				key += ":" + k + "=" + v
			}
		}
	}
	return key
}

// tokenCache provides thread-safe caching for installation tokens.
type tokenCache struct {
	cache            map[string]*InstallationToken
	mu               sync.RWMutex
	refreshThreshold time.Duration
}

func newTokenCache(refreshThreshold time.Duration) *tokenCache {
	return &tokenCache{
		cache:            make(map[string]*InstallationToken),
		refreshThreshold: refreshThreshold,
	}
}

func (c *tokenCache) get(key string) *InstallationToken {
	c.mu.RLock()
	defer c.mu.RUnlock()

	token, ok := c.cache[key]
	if !ok {
		return nil
	}

	// Check if token is still valid with buffer time
	if time.Until(token.ExpiresAt) < c.refreshThreshold {
		return nil
	}

	return token
}

func (c *tokenCache) set(key string, token *InstallationToken) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = token
}
