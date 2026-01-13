package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/boblangley/github-app-auth-container/internal/github"
	"github.com/boblangley/github-app-auth-container/internal/secrets"
	"github.com/boblangley/github-app-auth-container/internal/ssh"
)

const version = "1.0.0"

// Server holds all dependencies for the API handlers.
type Server struct {
	store         *secrets.Store
	githubService *github.Service
	signers       map[string]*ssh.Signer // Cached SSH signers per agent
}

// NewServer creates a new API server with the given dependencies.
func NewServer(store *secrets.Store, githubService *github.Service) *Server {
	return &Server{
		store:         store,
		githubService: githubService,
		signers:       make(map[string]*ssh.Signer),
	}
}

// HandleHealth returns the service health status.
func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	status := "healthy"
	if s.store.Count() == 0 {
		status = "degraded"
	}

	resp := HealthResponse{
		Status:     status,
		AgentCount: s.store.Count(),
		Version:    version,
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleToken returns a GitHub installation token for the requesting agent.
func (s *Server) HandleToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agent := GetAgentSecrets(ctx)
	agentName := GetAgentName(ctx)

	if agent == nil {
		writeError(w, http.StatusUnauthorized, "no agent credentials")
		return
	}

	// Parse optional request body for repository/permission scoping
	var tokenReq *github.TokenRequest
	if r.ContentLength > 0 {
		var req TokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		tokenReq = &github.TokenRequest{
			Repositories: req.Repositories,
			Permissions:  req.Permissions,
		}
	}

	// Get installation token
	token, err := s.githubService.GetInstallationToken(ctx, agent, tokenReq)
	if err != nil {
		slog.Error("failed to get installation token",
			"agent", agentName,
			"error", err,
		)
		writeError(w, http.StatusInternalServerError, "failed to get token")
		return
	}

	writeJSON(w, http.StatusOK, TokenResponse{
		Token:               token.Token,
		ExpiresAt:           token.ExpiresAt,
		Permissions:         token.Permissions,
		RepositorySelection: token.RepositorySelection,
	})
}

// HandleGitCredentials returns git credentials in credential helper format.
func (s *Server) HandleGitCredentials(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agent := GetAgentSecrets(ctx)
	agentName := GetAgentName(ctx)

	if agent == nil {
		writeError(w, http.StatusUnauthorized, "no agent credentials")
		return
	}

	// Parse request body
	var req GitCredentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate request
	if req.Protocol != "https" {
		writeError(w, http.StatusBadRequest, "only https protocol is supported")
		return
	}
	if req.Host != "github.com" && req.Host != "api.github.com" {
		writeError(w, http.StatusBadRequest, "only github.com is supported")
		return
	}

	// Get installation token
	token, err := s.githubService.GetInstallationToken(ctx, agent, nil)
	if err != nil {
		slog.Error("failed to get installation token",
			"agent", agentName,
			"error", err,
		)
		writeError(w, http.StatusInternalServerError, "failed to get credentials")
		return
	}

	writeJSON(w, http.StatusOK, GitCredentialsResponse{
		Protocol: req.Protocol,
		Host:     req.Host,
		Username: "x-access-token",
		Password: token.Token,
	})
}

// HandleIdentity returns the identity information for the requesting agent.
func (s *Server) HandleIdentity(w http.ResponseWriter, r *http.Request) {
	agent := GetAgentSecrets(r.Context())
	agentName := GetAgentName(r.Context())

	if agent == nil {
		writeError(w, http.StatusUnauthorized, "no agent credentials")
		return
	}

	// Get or create SSH signer for key ID
	signer, err := s.getOrCreateSigner(agentName, agent)
	if err != nil {
		slog.Warn("failed to create SSH signer", "agent", agentName, "error", err)
	}

	resp := IdentityResponse{
		AgentName: agentName,
	}
	resp.Identity.Name = agent.Identity.Name
	resp.Identity.Email = agent.Identity.Email

	if signer != nil {
		resp.SSHKeyID = signer.KeyID()
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleSSHSign signs data with the agent's SSH key.
func (s *Server) HandleSSHSign(w http.ResponseWriter, r *http.Request) {
	agent := GetAgentSecrets(r.Context())
	agentName := GetAgentName(r.Context())

	if agent == nil {
		writeError(w, http.StatusUnauthorized, "no agent credentials")
		return
	}

	// Parse request
	var req SSHSignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Data == "" {
		writeError(w, http.StatusBadRequest, "data is required")
		return
	}

	if req.Namespace == "" {
		writeError(w, http.StatusBadRequest, "namespace is required")
		return
	}

	// Get or create signer
	signer, err := s.getOrCreateSigner(agentName, agent)
	if err != nil {
		slog.Error("failed to create SSH signer", "agent", agentName, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create signer")
		return
	}

	// Sign the data
	signature, err := signer.SignGit([]byte(req.Data), req.Namespace)
	if err != nil {
		slog.Error("failed to sign data", "agent", agentName, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to sign data")
		return
	}

	resp := SSHSignResponse{
		Signature: signature,
		KeyID:     signer.KeyID(),
	}
	resp.SignerIdentity.Name = agent.Identity.Name
	resp.SignerIdentity.Email = agent.Identity.Email

	writeJSON(w, http.StatusOK, resp)
}

// HandleSSHPublicKey returns the SSH public key for the requesting agent.
func (s *Server) HandleSSHPublicKey(w http.ResponseWriter, r *http.Request) {
	agent := GetAgentSecrets(r.Context())
	agentName := GetAgentName(r.Context())

	if agent == nil {
		writeError(w, http.StatusUnauthorized, "no agent credentials")
		return
	}

	// Get or create signer
	signer, err := s.getOrCreateSigner(agentName, agent)
	if err != nil {
		slog.Error("failed to create SSH signer", "agent", agentName, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get public key")
		return
	}

	resp := SSHPublicKeyResponse{
		PublicKey: signer.PublicKey(),
		KeyID:     signer.KeyID(),
	}
	resp.Identity.Name = agent.Identity.Name
	resp.Identity.Email = agent.Identity.Email

	writeJSON(w, http.StatusOK, resp)
}

// getOrCreateSigner returns a cached SSH signer or creates a new one.
func (s *Server) getOrCreateSigner(agentName string, agent *secrets.AgentSecrets) (*ssh.Signer, error) {
	if signer, ok := s.signers[agentName]; ok {
		return signer, nil
	}

	signer, err := ssh.NewSigner(agent.PrivateKey)
	if err != nil {
		return nil, err
	}

	s.signers[agentName] = signer
	return signer, nil
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

// writeError writes an error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}
