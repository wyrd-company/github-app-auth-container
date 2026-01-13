// git-credential-github-app is a git credential helper that retrieves
// GitHub App installation tokens from the auth service.
//
// Usage:
//   git config --global credential.helper github-app
//
// Required environment variables:
//   GITHUB_AUTH_SERVICE - URL of the auth service (e.g., http://github-auth-service:8080)
//   AGENT_NAME          - Agent identifier
//   AGENT_TOKEN         - Pre-signed authentication token
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type credentialRequest struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Path     string `json:"path,omitempty"`
}

type credentialResponse struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: git-credential-github-app <get|store|erase>")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "get":
		if err := handleGet(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "store", "erase":
		// Nothing to do - tokens are managed by the auth service
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown operation: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func handleGet() error {
	// Read credential request from stdin
	input := parseInput()

	protocol := input["protocol"]
	host := input["host"]
	path := input["path"]

	// Only handle GitHub HTTPS requests
	if protocol != "https" {
		return fmt.Errorf("only https protocol is supported")
	}
	if host != "github.com" && host != "api.github.com" {
		return fmt.Errorf("only github.com hosts are supported")
	}

	// Get configuration from environment
	authService := os.Getenv("GITHUB_AUTH_SERVICE")
	if authService == "" {
		return fmt.Errorf("GITHUB_AUTH_SERVICE environment variable is required")
	}
	agentName := os.Getenv("AGENT_NAME")
	if agentName == "" {
		return fmt.Errorf("AGENT_NAME environment variable is required")
	}
	agentToken := os.Getenv("AGENT_TOKEN")
	if agentToken == "" {
		return fmt.Errorf("AGENT_TOKEN environment variable is required")
	}

	// Request credentials from auth service
	creds, err := getCredentials(authService, agentName, agentToken, credentialRequest{
		Protocol: protocol,
		Host:     host,
		Path:     path,
	})
	if err != nil {
		return err
	}

	// Output in git credential format
	fmt.Printf("protocol=%s\n", creds.Protocol)
	fmt.Printf("host=%s\n", creds.Host)
	fmt.Printf("username=%s\n", creds.Username)
	fmt.Printf("password=%s\n", creds.Password)

	return nil
}

func parseInput() map[string]string {
	result := make(map[string]string)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		if idx := strings.Index(line, "="); idx > 0 {
			key := line[:idx]
			value := line[idx+1:]
			result[key] = value
		}
	}
	return result
}

func getCredentials(authService, agentName, agentToken string, req credentialRequest) (*credentialResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimSuffix(authService, "/") + "/api/v1/git-credentials"
	httpReq, err := http.NewRequest("POST", url, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Agent-Name", agentName)
	httpReq.Header.Set("X-Agent-Token", agentToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to contact auth service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Message != "" {
			return nil, fmt.Errorf("auth service error: %s", errResp.Message)
		}
		return nil, fmt.Errorf("auth service returned status %d", resp.StatusCode)
	}

	var creds credentialResponse
	if err := json.NewDecoder(resp.Body).Decode(&creds); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &creds, nil
}
