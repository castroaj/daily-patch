// config.go — configuration loading for the api service
//
// Reads config.yaml, overlays environment variables for secrets, and
// validates required fields. Call Load once at startup; fatal on error.

package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

const (
	defaultListen     = ":8080"
	envInternalSecret = "API_INTERNAL_SECRET"
	envDatabaseURL    = "DATABASE_URL"
)

// -----------------------------------------------------------------------------
// Types
// -----------------------------------------------------------------------------

// Config holds all configuration for the api service.
type Config struct {
	API      APIConfig      `yaml:"api"`
	Database DatabaseConfig `yaml:"database"`
}

// APIConfig holds HTTP server and auth settings.
type APIConfig struct {
	Listen         string `yaml:"listen"`          // default ":8080"
	InternalSecret string `yaml:"internal_secret"` // env: API_INTERNAL_SECRET
}

// DatabaseConfig holds the database connection string.
type DatabaseConfig struct {
	DSN string `yaml:"dsn"` // env: DATABASE_URL
}

// -----------------------------------------------------------------------------
// Public functions
// -----------------------------------------------------------------------------

// Load reads the YAML file at path, overlays environment variables, applies
// defaults, and validates required fields. Returns an error if the file is
// missing or malformed, or if a required field is empty after the overlay.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config file: %w", err)
	}

	applyEnv(&cfg)
	applyDefaults(&cfg)

	if err := validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// -----------------------------------------------------------------------------
// Private functions
// -----------------------------------------------------------------------------

// applyEnv overlays non-empty environment variables onto cfg.
func applyEnv(cfg *Config) {
	if v := os.Getenv(envInternalSecret); v != "" {
		cfg.API.InternalSecret = v
	}
	if v := os.Getenv(envDatabaseURL); v != "" {
		cfg.Database.DSN = v
	}
}

// applyDefaults sets fields to their default values when absent.
func applyDefaults(cfg *Config) {
	if cfg.API.Listen == "" {
		cfg.API.Listen = defaultListen
	}
}

// validate returns an error if any required field is empty.
func validate(cfg Config) error {
	if cfg.API.InternalSecret == "" {
		return fmt.Errorf("required field API_INTERNAL_SECRET (api.internal_secret) is not set")
	}
	if cfg.Database.DSN == "" {
		return fmt.Errorf("required field DATABASE_URL (database.dsn) is not set")
	}
	return nil
}
