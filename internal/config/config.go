package config

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration, loaded from environment variables.
type Config struct {
	// Server settings
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// Secrets backend selection: "bitwarden" (default) or "vault"
	SecretsBackend string

	// Bitwarden settings (used when SecretsBackend == "bitwarden")
	BitwardenServerURL string
	CollectionID       string

	// HashiCorp Vault settings (used when SecretsBackend == "vault")
	VaultAddr      string
	VaultMountPath string
	VaultBasePath  string

	// GitHub API settings
	GitHubAPIURL string

	// Logging
	LogLevel string
}

// Load reads configuration from environment variables.
func Load() *Config {
	cfg := &Config{
		Port:               getEnvInt("PORT", 8080),
		ReadTimeout:        getEnvDuration("READ_TIMEOUT", 30*time.Second),
		WriteTimeout:       getEnvDuration("WRITE_TIMEOUT", 30*time.Second),
		SecretsBackend:     getEnvString("SECRETS_BACKEND", "bitwarden"),
		BitwardenServerURL: os.Getenv("BW_SERVER_URL"),
		CollectionID:       getEnvOrFile("BW_COLLECTION_ID", "BW_COLLECTION_ID_FILE"),
		VaultAddr:          os.Getenv("VAULT_ADDR"),
		VaultMountPath:     getEnvString("VAULT_MOUNT_PATH", "secret"),
		VaultBasePath:      getEnvString("VAULT_BASE_PATH", "agents"),
		GitHubAPIURL:       getEnvString("GITHUB_API_URL", "https://api.github.com"),
		LogLevel:           getEnvString("LOG_LEVEL", "INFO"),
	}

	return cfg
}

// getEnvOrFile reads from env var, or from a file if the _FILE variant is set.
func getEnvOrFile(envKey, fileEnvKey string) string {
	if val := os.Getenv(envKey); val != "" {
		return val
	}
	if filePath := os.Getenv(fileEnvKey); filePath != "" {
		data, err := os.ReadFile(filePath)
		if err == nil {
			return string(bytes.TrimSpace(data))
		}
	}
	return ""
}

// Validate checks that required configuration is present.
func (c *Config) Validate() error {
	switch c.SecretsBackend {
	case "bitwarden":
		if c.CollectionID == "" {
			return &ConfigError{Field: "BW_COLLECTION_ID", Message: "required when SECRETS_BACKEND=bitwarden"}
		}
	case "vault":
		if c.VaultBasePath == "" {
			return &ConfigError{Field: "VAULT_BASE_PATH", Message: "required when SECRETS_BACKEND=vault"}
		}
	default:
		return &ConfigError{
			Field:   "SECRETS_BACKEND",
			Message: fmt.Sprintf("unsupported backend: %s (must be 'bitwarden' or 'vault')", c.SecretsBackend),
		}
	}
	return nil
}

// ConfigError represents a configuration error.
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return e.Field + ": " + e.Message
}

func getEnvString(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
