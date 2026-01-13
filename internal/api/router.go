package api

import (
	"net/http"

	"github.com/boblangley/github-app-auth-container/internal/secrets"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Router returns the HTTP router for the API.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RealIP)
	r.Use(RecoveryMiddleware)
	r.Use(RequestLoggingMiddleware)
	r.Use(middleware.RequestID)

	// Health check - no auth required
	r.Get("/health", s.HandleHealth)

	// API routes - require token authentication
	r.Route("/api/v1", func(r chi.Router) {
		// Apply token auth middleware to all API routes
		r.Use(TokenAuthMiddleware(s.store))

		// Token endpoints
		r.Get("/token", s.HandleToken)
		r.Post("/token", s.HandleToken)

		// Git credentials endpoint
		r.Post("/git-credentials", s.HandleGitCredentials)

		// Identity endpoint
		r.Get("/identity", s.HandleIdentity)

		// SSH signing endpoints (replaces GPG)
		r.Route("/ssh", func(r chi.Router) {
			r.Post("/sign", s.HandleSSHSign)
			r.Get("/public-key", s.HandleSSHPublicKey)
		})
	})

	return r
}

// Store returns the secrets store (for middleware).
func (s *Server) Store() *secrets.Store {
	return s.store
}
