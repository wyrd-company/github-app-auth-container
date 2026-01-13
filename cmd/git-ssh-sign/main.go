// git-ssh-sign is an SSH signing program for git that uses the auth service.
//
// Usage (git configuration):
//   git config --global gpg.format ssh
//   git config --global gpg.ssh.program git-ssh-sign
//   git config --global user.signingkey "(managed by auth service)"
//
// Required environment variables:
//   GITHUB_AUTH_SERVICE - URL of the auth service (e.g., http://github-auth-service:8080)
//   AGENT_NAME          - Agent identifier
//   AGENT_TOKEN         - Pre-signed authentication token
//
// Git calls this program like ssh-keygen:
//   git-ssh-sign -Y sign -f <key_file> -n <namespace> < data
//
// This helper ignores the key file (-f) since the auth service manages keys.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type signRequest struct {
	Data      string `json:"data"`
	Namespace string `json:"namespace"`
}

type signResponse struct {
	Signature      string `json:"signature"`
	KeyID          string `json:"key_id"`
	SignerIdentity struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"signer_identity"`
}

type publicKeyResponse struct {
	PublicKey string `json:"public_key"`
	KeyID     string `json:"key_id"`
	Identity  struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"identity"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func main() {
	// Parse flags similar to ssh-keygen
	var (
		operation string
		keyFile   string
		namespace string
	)

	flag.StringVar(&operation, "Y", "", "Operation: sign, find-principals, check-novalidate, verify")
	flag.StringVar(&keyFile, "f", "", "Key file (ignored - managed by auth service)")
	flag.StringVar(&namespace, "n", "git", "Namespace for signing")
	flag.Parse()

	switch operation {
	case "sign":
		if err := handleSign(namespace); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "find-principals":
		// Used for verifying signatures - return the identity
		if err := handleFindPrincipals(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "check-novalidate":
		// Verify signature format without checking trust
		// We can't do this without the public key, so just exit success
		os.Exit(0)
	case "verify":
		// Full verification - would need allowed signers file
		fmt.Fprintln(os.Stderr, "verify operation not supported - configure allowed_signers file")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "unknown or missing operation: %s\n", operation)
		fmt.Fprintln(os.Stderr, "usage: git-ssh-sign -Y sign [-n namespace] < data")
		os.Exit(1)
	}
}

func handleSign(namespace string) error {
	// Read data to sign from stdin
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
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

	// Request signature from auth service
	sig, err := requestSignature(authService, agentName, agentToken, signRequest{
		Data:      string(data),
		Namespace: namespace,
	})
	if err != nil {
		return err
	}

	// Output signature to stdout (git expects this format)
	fmt.Print(sig.Signature)

	return nil
}

func handleFindPrincipals() error {
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

	// Get public key info from auth service
	keyInfo, err := getPublicKey(authService, agentName, agentToken)
	if err != nil {
		return err
	}

	// Output the identity email as the principal
	fmt.Println(keyInfo.Identity.Email)

	return nil
}

func requestSignature(authService, agentName, agentToken string, req signRequest) (*signResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimSuffix(authService, "/") + "/api/v1/ssh/sign"
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

	var sig signResponse
	if err := json.NewDecoder(resp.Body).Decode(&sig); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &sig, nil
}

func getPublicKey(authService, agentName, agentToken string) (*publicKeyResponse, error) {
	url := strings.TrimSuffix(authService, "/") + "/api/v1/ssh/public-key"
	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

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

	var keyInfo publicKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&keyInfo); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &keyInfo, nil
}
