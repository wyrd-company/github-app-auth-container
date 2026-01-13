package config

import (
	"bytes"
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

	// Bitwarden settings
	BitwardenServerURL string
	CollectionID       string

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
		BitwardenServerURL: os.Getenv("BW_SERVER_URL"),
		CollectionID:       getEnvOrFile("BW_COLLECTION_ID", "BW_COLLECTION_ID_FILE"),
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
	if c.CollectionID == "" {
		return &ConfigError{Field: "BW_COLLECTION_ID", Message: "required"}
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
