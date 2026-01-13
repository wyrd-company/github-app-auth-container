// gh-github-app is a wrapper for the GitHub CLI that automatically
// fetches an installation token from the auth service.
//
// Usage:
//   gh-github-app repo list
//   gh-github-app pr create --title "Fix bug" --body "..."
//
// Or alias it:
//   alias gh='gh-github-app'
//
// Required environment variables:
//   GITHUB_AUTH_SERVICE - URL of the auth service (e.g., http://github-auth-service:8080)
//   AGENT_NAME          - Agent identifier
//   AGENT_TOKEN         - Pre-signed authentication token
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type tokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "gh-github-app: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
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

	// Fetch token from auth service
	token, err := getToken(authService, agentName, agentToken)
	if err != nil {
		return fmt.Errorf("failed to get token: %w", err)
	}

	// Find gh binary - check known locations first to avoid alias loops
	ghPath := findGhBinary()
	if ghPath == "" {
		return fmt.Errorf("gh CLI not found - install it from https://cli.github.com")
	}

	// Build environment with GH_TOKEN set
	env := os.Environ()
	env = setEnv(env, "GH_TOKEN", token)

	// Exec gh with all arguments
	// Using syscall.Exec replaces this process with gh
	args := append([]string{"gh"}, os.Args[1:]...)
	return syscall.Exec(ghPath, args, env)
}

func getToken(authService, agentName, agentToken string) (string, error) {
	url := strings.TrimSuffix(authService, "/") + "/api/v1/token"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Agent-Name", agentName)
	req.Header.Set("X-Agent-Token", agentToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to contact auth service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Message != "" {
			return "", fmt.Errorf("auth service error: %s", errResp.Message)
		}
		return "", fmt.Errorf("auth service returned status %d", resp.StatusCode)
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return tokenResp.Token, nil
}

// findGhBinary looks for the gh binary in known locations.
// Checks specific paths first to avoid alias/symlink loops.
func findGhBinary() string {
	// Check known installation paths first
	knownPaths := []string{
		"/usr/bin/gh-real", // If renamed to allow symlink replacement
		"/usr/bin/gh",
		"/usr/local/bin/gh",
		"/opt/homebrew/bin/gh",
		"/home/linuxbrew/.linuxbrew/bin/gh",
	}

	for _, p := range knownPaths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}

	// Fall back to PATH lookup, but skip ourselves
	if path, err := exec.LookPath("gh"); err == nil {
		// Make sure it's not a symlink to us
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			if !strings.HasSuffix(resolved, "gh-github-app") {
				return path
			}
		}
	}

	return ""
}

// setEnv sets or replaces an environment variable in the env slice.
func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
