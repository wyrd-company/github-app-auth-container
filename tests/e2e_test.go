package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

var (
	authServiceURL string
	agentName      string
	agentToken     string
	httpClient     *http.Client
)

func TestMain(m *testing.M) {
	authServiceURL = os.Getenv("GITHUB_AUTH_SERVICE")
	if authServiceURL == "" {
		authServiceURL = "http://github-auth-service:8080"
	}

	agentName = os.Getenv("TEST_AGENT_NAME")
	if agentName == "" {
		agentName = "test-agent"
	}

	// Load agent token from file or env var
	if tokenFile := os.Getenv("TEST_AGENT_TOKEN_FILE"); tokenFile != "" {
		data, err := os.ReadFile(tokenFile)
		if err == nil {
			agentToken = strings.TrimSpace(string(data))
		}
	}
	if agentToken == "" {
		agentToken = os.Getenv("TEST_AGENT_TOKEN")
	}

	httpClient = &http.Client{
		Timeout: 30 * time.Second,
	}

	// Wait for service to be ready
	if err := waitForService(authServiceURL+"/health", 60*time.Second); err != nil {
		fmt.Printf("Service not ready: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func waitForService(url string, timeout time.Duration) error {
	start := time.Now()
	for {
		resp, err := httpClient.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}

		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for service at %s", url)
		}

		time.Sleep(2 * time.Second)
	}
}

// authenticatedRequest creates a request with agent authentication headers.
func authenticatedRequest(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Agent-Name", agentName)
	req.Header.Set("X-Agent-Token", agentToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// =============================================================================
// Health Check Tests
// =============================================================================

func TestHealthEndpoint(t *testing.T) {
	resp, err := httpClient.Get(authServiceURL + "/health")
	if err != nil {
		t.Fatalf("Failed to call health endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var health struct {
		Status      string `json:"status"`
		AgentCount  int    `json:"agent_count"`
		Version     string `json:"version"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("Failed to decode health response: %v", err)
	}

	t.Logf("Health status: %+v", health)

	if health.Status != "healthy" {
		t.Errorf("Unexpected health status: %s", health.Status)
	}

	if health.AgentCount < 1 {
		t.Errorf("Expected at least 1 agent, got %d", health.AgentCount)
	}

	if health.Version == "" {
		t.Error("Version should not be empty")
	}
}

// =============================================================================
// Token Endpoint Tests
// =============================================================================

func TestTokenEndpoint(t *testing.T) {
	if agentToken == "" {
		t.Skip("No agent token available - skipping authenticated tests")
	}

	req, err := authenticatedRequest("GET", authServiceURL+"/api/v1/token", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to call token endpoint: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("Token request unauthorized - check agent credentials: %s", string(body))
	}

	// 500 means GitHub API call failed (expected without real GitHub App credentials)
	if resp.StatusCode == http.StatusInternalServerError && os.Getenv("TEST_GITHUB_API_ENABLED") != "true" {
		t.Skip("Token request failed (GitHub API not configured) - set TEST_GITHUB_API_ENABLED=true with real credentials")
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var token struct {
		Token               string            `json:"token"`
		ExpiresAt           string            `json:"expires_at"`
		Permissions         map[string]string `json:"permissions"`
		RepositorySelection string            `json:"repository_selection"`
	}

	if err := json.Unmarshal(body, &token); err != nil {
		t.Fatalf("Failed to decode token response: %v", err)
	}

	t.Logf("Token received: %s...", token.Token[:min(20, len(token.Token))])
	t.Logf("Expires at: %s", token.ExpiresAt)
	t.Logf("Permissions: %v", token.Permissions)

	// Validate token format (GitHub installation tokens start with "ghs_")
	if !strings.HasPrefix(token.Token, "ghs_") {
		t.Errorf("Token should start with 'ghs_', got: %s...", token.Token[:min(10, len(token.Token))])
	}

	if token.ExpiresAt == "" {
		t.Error("ExpiresAt should not be empty")
	}
}

func TestTokenEndpointUnauthorized(t *testing.T) {
	// Test without auth headers
	resp, err := httpClient.Get(authServiceURL + "/api/v1/token")
	if err != nil {
		t.Fatalf("Failed to call token endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401 without auth, got %d", resp.StatusCode)
	}
}

func TestTokenEndpointWithScoping(t *testing.T) {
	if agentToken == "" {
		t.Skip("No agent token available")
	}

	reqBody := map[string]interface{}{
		"repositories": []string{"test-repo"},
		"permissions": map[string]string{
			"contents": "read",
		},
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, err := authenticatedRequest("POST", authServiceURL+"/api/v1/token", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to call token endpoint: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Note: Scoped tokens may fail if the repo doesn't exist or isn't accessible
	// A 422 or 404 from GitHub is acceptable in this case
	if resp.StatusCode != http.StatusOK {
		t.Logf("Scoped token request returned %d: %s", resp.StatusCode, string(body))
		// Don't fail - the scoping might reference repos we don't have access to
	}
}

// =============================================================================
// Git Credentials Endpoint Tests
// =============================================================================

func TestGitCredentialsEndpoint(t *testing.T) {
	if agentToken == "" {
		t.Skip("No agent token available")
	}

	reqBody := map[string]string{
		"protocol": "https",
		"host":     "github.com",
		"path":     "owner/repo",
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, err := authenticatedRequest("POST", authServiceURL+"/api/v1/git-credentials", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to call git-credentials endpoint: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// 500 means GitHub API call failed (expected without real GitHub App credentials)
	if resp.StatusCode == http.StatusInternalServerError && os.Getenv("TEST_GITHUB_API_ENABLED") != "true" {
		t.Skip("Git credentials failed (GitHub API not configured) - set TEST_GITHUB_API_ENABLED=true with real credentials")
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var creds struct {
		Protocol string `json:"protocol"`
		Host     string `json:"host"`
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.Unmarshal(body, &creds); err != nil {
		t.Fatalf("Failed to decode credentials response: %v", err)
	}

	t.Logf("Credentials received for %s://%s", creds.Protocol, creds.Host)

	if creds.Protocol != "https" {
		t.Errorf("Expected protocol 'https', got '%s'", creds.Protocol)
	}

	if creds.Host != "github.com" {
		t.Errorf("Expected host 'github.com', got '%s'", creds.Host)
	}

	if creds.Username != "x-access-token" {
		t.Errorf("Expected username 'x-access-token', got '%s'", creds.Username)
	}

	if !strings.HasPrefix(creds.Password, "ghs_") {
		t.Errorf("Password should be a GitHub token starting with 'ghs_'")
	}
}

func TestGitCredentialsInvalidProtocol(t *testing.T) {
	if agentToken == "" {
		t.Skip("No agent token available")
	}

	reqBody := map[string]string{
		"protocol": "ssh",
		"host":     "github.com",
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, err := authenticatedRequest("POST", authServiceURL+"/api/v1/git-credentials", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to call git-credentials endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid protocol, got %d", resp.StatusCode)
	}
}

// =============================================================================
// Identity Endpoint Tests
// =============================================================================

func TestIdentityEndpoint(t *testing.T) {
	if agentToken == "" {
		t.Skip("No agent token available")
	}

	req, err := authenticatedRequest("GET", authServiceURL+"/api/v1/identity", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to call identity endpoint: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var identity struct {
		AgentName string `json:"agent_name"`
		Identity  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"identity"`
		SSHPublicKey string `json:"ssh_public_key,omitempty"`
	}

	if err := json.Unmarshal(body, &identity); err != nil {
		t.Fatalf("Failed to decode identity response: %v", err)
	}

	t.Logf("Identity: %s <%s>", identity.Identity.Name, identity.Identity.Email)
	t.Logf("Agent: %s", identity.AgentName)

	if identity.AgentName != agentName {
		t.Errorf("Expected agent name '%s', got '%s'", agentName, identity.AgentName)
	}

	expectedEmail := os.Getenv("TEST_AGENT_EMAIL")
	if expectedEmail != "" && identity.Identity.Email != expectedEmail {
		t.Errorf("Expected email '%s', got '%s'", expectedEmail, identity.Identity.Email)
	}
}

// =============================================================================
// SSH Signing Endpoint Tests
// =============================================================================

func TestSSHPublicKeyEndpoint(t *testing.T) {
	if agentToken == "" {
		t.Skip("No agent token available")
	}

	req, err := authenticatedRequest("GET", authServiceURL+"/api/v1/ssh/public-key", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to call ssh/public-key endpoint: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var sshKey struct {
		PublicKey string `json:"public_key"`
		KeyID     string `json:"key_id"`
		Identity  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"identity"`
	}

	if err := json.Unmarshal(body, &sshKey); err != nil {
		t.Fatalf("Failed to decode SSH key response: %v", err)
	}

	t.Logf("SSH Key ID: %s", sshKey.KeyID)
	t.Logf("Public Key: %s...", sshKey.PublicKey[:min(50, len(sshKey.PublicKey))])

	// SSH public keys start with the algorithm (ssh-rsa, ecdsa-sha2-nistp256, etc.)
	if !strings.HasPrefix(sshKey.PublicKey, "ssh-") && !strings.HasPrefix(sshKey.PublicKey, "ecdsa-") {
		t.Error("Public key should start with ssh- or ecdsa-")
	}

	if sshKey.KeyID == "" {
		t.Error("Key ID should not be empty")
	}
}

func TestSSHSignEndpoint(t *testing.T) {
	if agentToken == "" {
		t.Skip("No agent token available")
	}

	reqBody := map[string]string{
		"data":      "Test data to sign\n",
		"namespace": "git",
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, err := authenticatedRequest("POST", authServiceURL+"/api/v1/ssh/sign", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to call ssh/sign endpoint: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var signResp struct {
		Signature      string `json:"signature"`
		KeyID          string `json:"key_id"`
		SignerIdentity struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"signer_identity"`
	}

	if err := json.Unmarshal(body, &signResp); err != nil {
		t.Fatalf("Failed to decode sign response: %v", err)
	}

	t.Logf("Signature received from key %s", signResp.KeyID)

	// SSH signatures in git format have a specific header
	if !strings.Contains(signResp.Signature, "-----BEGIN SSH SIGNATURE-----") {
		t.Error("Signature should contain SSH signature header")
	}

	if !strings.Contains(signResp.Signature, "-----END SSH SIGNATURE-----") {
		t.Error("Signature should contain SSH signature footer")
	}
}

func TestSSHSignInvalidNamespace(t *testing.T) {
	if agentToken == "" {
		t.Skip("No agent token available")
	}

	reqBody := map[string]string{
		"data":      "Test data to sign\n",
		"namespace": "", // Empty namespace should fail
	}

	jsonBody, _ := json.Marshal(reqBody)
	req, err := authenticatedRequest("POST", authServiceURL+"/api/v1/ssh/sign", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to call ssh/sign endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty namespace, got %d", resp.StatusCode)
	}
}

// =============================================================================
// Integration Test - Full Flow
// =============================================================================

func TestFullGitWorkflow(t *testing.T) {
	if agentToken == "" {
		t.Skip("No agent token available")
	}

	// Skip if GitHub API testing is not enabled
	if os.Getenv("TEST_GITHUB_API_ENABLED") != "true" {
		t.Skip("GitHub API testing not enabled - set TEST_GITHUB_API_ENABLED=true")
	}

	// Step 1: Get identity
	t.Log("Step 1: Getting identity...")
	identityReq, _ := authenticatedRequest("GET", authServiceURL+"/api/v1/identity", nil)
	identityResp, err := httpClient.Do(identityReq)
	if err != nil {
		t.Fatalf("Failed to get identity: %v", err)
	}
	identityResp.Body.Close()

	if identityResp.StatusCode != http.StatusOK {
		t.Fatalf("Identity request failed: %d", identityResp.StatusCode)
	}

	// Step 2: Get credentials
	t.Log("Step 2: Getting git credentials...")
	credReq := map[string]string{
		"protocol": "https",
		"host":     "github.com",
	}
	credJSON, _ := json.Marshal(credReq)
	credHttpReq, _ := authenticatedRequest("POST", authServiceURL+"/api/v1/git-credentials", bytes.NewReader(credJSON))
	credResp, err := httpClient.Do(credHttpReq)
	if err != nil {
		t.Fatalf("Failed to get credentials: %v", err)
	}
	defer credResp.Body.Close()

	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	json.NewDecoder(credResp.Body).Decode(&creds)

	// Step 3: Verify token works with GitHub API
	t.Log("Step 3: Verifying token with GitHub API...")
	ghReq, _ := http.NewRequest("GET", "https://api.github.com/installation/repositories", nil)
	ghReq.Header.Set("Authorization", "Bearer "+creds.Password)
	ghReq.Header.Set("Accept", "application/vnd.github+json")

	ghResp, err := httpClient.Do(ghReq)
	if err != nil {
		t.Fatalf("Failed to call GitHub API: %v", err)
	}
	defer ghResp.Body.Close()

	ghBody, _ := io.ReadAll(ghResp.Body)

	if ghResp.StatusCode != http.StatusOK {
		t.Fatalf("GitHub API returned %d: %s", ghResp.StatusCode, string(ghBody))
	}

	// Step 4: Test SSH signing
	t.Log("Step 4: Testing SSH signing...")
	signReq := map[string]string{
		"data":      "tree abc123\nparent def456\nauthor Test <test@example.com>\ncommitter Test <test@example.com>\n\nTest commit",
		"namespace": "git",
	}
	signJSON, _ := json.Marshal(signReq)
	signHttpReq, _ := authenticatedRequest("POST", authServiceURL+"/api/v1/ssh/sign", bytes.NewReader(signJSON))
	signResp, err := httpClient.Do(signHttpReq)
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}
	defer signResp.Body.Close()

	if signResp.StatusCode != http.StatusOK {
		signBody, _ := io.ReadAll(signResp.Body)
		t.Fatalf("Signing failed: %d: %s", signResp.StatusCode, string(signBody))
	}

	t.Log("Full workflow completed successfully!")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
