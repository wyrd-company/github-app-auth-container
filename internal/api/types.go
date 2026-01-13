package api

import "time"

// HealthResponse is returned by the health check endpoint.
type HealthResponse struct {
	Status     string `json:"status"`
	AgentCount int    `json:"agent_count"`
	Version    string `json:"version"`
}

// TokenResponse is returned by the token endpoint.
type TokenResponse struct {
	Token               string            `json:"token"`
	ExpiresAt           time.Time         `json:"expires_at"`
	Permissions         map[string]string `json:"permissions"`
	RepositorySelection string            `json:"repository_selection"`
}

// TokenRequest is the optional request body for the token endpoint.
type TokenRequest struct {
	Repositories []string          `json:"repositories,omitempty"`
	Permissions  map[string]string `json:"permissions,omitempty"`
}

// GitCredentialsRequest is the request body for the git credentials endpoint.
type GitCredentialsRequest struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Path     string `json:"path,omitempty"`
}

// GitCredentialsResponse is returned by the git credentials endpoint.
type GitCredentialsResponse struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// IdentityResponse is returned by the identity endpoint.
type IdentityResponse struct {
	AgentName string `json:"agent_name"`
	Identity  struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"identity"`
	SSHKeyID string `json:"ssh_key_id,omitempty"`
}

// SSHSignRequest is the request body for the SSH sign endpoint.
type SSHSignRequest struct {
	Data      string `json:"data"`
	Namespace string `json:"namespace,omitempty"` // Default: "git"
}

// SSHSignResponse is returned by the SSH sign endpoint.
type SSHSignResponse struct {
	Signature      string `json:"signature"`
	KeyID          string `json:"key_id"`
	SignerIdentity struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"signer_identity"`
}

// SSHPublicKeyResponse is returned by the SSH public key endpoint.
type SSHPublicKeyResponse struct {
	PublicKey string `json:"public_key"`
	KeyID     string `json:"key_id"`
	Identity  struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"identity"`
}

// ErrorResponse is returned when an error occurs.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}
