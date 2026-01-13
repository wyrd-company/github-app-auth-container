package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/boblangley/github-app-auth-container/internal/secrets"
)

// contextKey is used for context values.
type contextKey string

const (
	// AgentNameKey is the context key for the agent name.
	AgentNameKey contextKey = "agentName"
	// AgentSecretsKey is the context key for the agent secrets.
	AgentSecretsKey contextKey = "agentSecrets"
)

// TokenAuthMiddleware authenticates requests using signed agent tokens.
// Agents must provide their name and token in headers:
//   - X-Agent-Name: agent-claude-1
//   - X-Agent-Token: <base64 signature>
func TokenAuthMiddleware(store *secrets.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract agent credentials from headers
			agentName := r.Header.Get("X-Agent-Name")
			agentToken := r.Header.Get("X-Agent-Token")

			if agentName == "" || agentToken == "" {
				slog.Debug("missing agent credentials",
					"has_name", agentName != "",
					"has_token", agentToken != "",
				)
				writeError(w, http.StatusUnauthorized, "missing X-Agent-Name or X-Agent-Token header")
				return
			}

			// Verify the token
			if !store.VerifyAgentToken(agentName, agentToken) {
				slog.Warn("invalid agent token", "agent", agentName)
				writeError(w, http.StatusUnauthorized, "invalid agent token")
				return
			}

			// Get agent secrets
			agentSecrets, ok := store.GetAgent(agentName)
			if !ok {
				slog.Warn("agent not found", "agent", agentName)
				writeError(w, http.StatusUnauthorized, "agent not found")
				return
			}

			// Add to context
			ctx := context.WithValue(r.Context(), AgentNameKey, agentName)
			ctx = context.WithValue(ctx, AgentSecretsKey, agentSecrets)

			slog.Debug("authenticated agent",
				"agent", agentName,
				"path", r.URL.Path,
			)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequestLoggingMiddleware logs HTTP requests.
func RequestLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		// Don't log health checks at info level
		logLevel := slog.LevelInfo
		if strings.HasPrefix(r.URL.Path, "/health") {
			logLevel = slog.LevelDebug
		}

		slog.Log(r.Context(), logLevel, "http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration", duration,
		)
	})
}

// RecoveryMiddleware recovers from panics and returns a 500 error.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered", "error", err, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// GetAgentName retrieves the agent name from the request context.
func GetAgentName(ctx context.Context) string {
	if name, ok := ctx.Value(AgentNameKey).(string); ok {
		return name
	}
	return ""
}

// GetAgentSecrets retrieves the agent secrets from the request context.
func GetAgentSecrets(ctx context.Context) *secrets.AgentSecrets {
	if s, ok := ctx.Value(AgentSecretsKey).(*secrets.AgentSecrets); ok {
		return s
	}
	return nil
}
