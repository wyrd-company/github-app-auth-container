package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/boblangley/github-app-auth-container/internal/api"
	"github.com/boblangley/github-app-auth-container/internal/config"
	"github.com/boblangley/github-app-auth-container/internal/github"
	"github.com/boblangley/github-app-auth-container/internal/secrets"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Set up structured logging
	logLevel := slog.LevelInfo
	if cfg.LogLevel == "DEBUG" {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	slog.Info("starting github-app-auth-container")

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	// Initialize secrets backend
	var backend secrets.SecretsBackend
	var scope string
	var err error

	switch cfg.SecretsBackend {
	case "bitwarden":
		slog.Info("connecting to Bitwarden...")
		backend, err = secrets.NewBitwardenBackend(cfg.BitwardenServerURL)
		scope = cfg.CollectionID
	case "vault":
		slog.Info("connecting to HashiCorp Vault...")
		backend, err = secrets.NewVaultBackend(cfg.VaultAddr, cfg.VaultMountPath)
		scope = cfg.VaultBasePath
	}

	if err != nil {
		slog.Error("failed to initialize secrets backend",
			"backend", cfg.SecretsBackend,
			"error", err,
		)
		os.Exit(1)
	}

	// Create secrets store and load all agents
	store := secrets.NewStore()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	if err := store.LoadFromBackend(ctx, backend, scope); err != nil {
		cancel()
		slog.Error("failed to load secrets",
			"backend", cfg.SecretsBackend,
			"error", err,
		)
		os.Exit(1)
	}
	cancel()

	slog.Info("secrets loaded successfully",
		"agents", store.Count(),
		"agent_names", store.ListAgents(),
	)

	// Initialize GitHub service
	githubService := github.NewService(cfg.GitHubAPIURL)

	// Create API server
	server := api.NewServer(store, githubService)

	// Set up HTTP server
	addr := fmt.Sprintf(":%d", cfg.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      server.Router(),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		slog.Info("starting HTTP server", "address", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
	}

	slog.Info("server stopped")
}
