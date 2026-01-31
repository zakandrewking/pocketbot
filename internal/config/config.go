package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the pocketbot configuration
type Config struct {
	Claude   ClaudeConfig    `yaml:"claude"`
	Sessions []SessionConfig `yaml:"sessions"`
}

// ClaudeConfig represents the Claude session configuration
type ClaudeConfig struct {
	Command string `yaml:"command"`
	Key     string `yaml:"key"`
	Enabled bool   `yaml:"enabled"`
}

// SessionConfig represents a custom session configuration
type SessionConfig struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
	Key     string `yaml:"key"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Claude: ClaudeConfig{
			Command: "claude --continue --accept-edits",
			Key:     "c",
			Enabled: true,
		},
		Sessions: []SessionConfig{},
	}
}

// ConfigPath returns the path to the config file
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".config", "pocketbot", "config.yaml"), nil
}

// Load loads the configuration from the config file
// If the file doesn't exist, returns the default config
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	// If config file doesn't exist, return default
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	// Read config file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults for missing fields
	if cfg.Claude.Command == "" {
		cfg.Claude.Command = "claude --continue --accept-edits"
	}
	if cfg.Claude.Key == "" {
		cfg.Claude.Key = "c"
	}

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Check for duplicate keys
	keys := make(map[string]string)

	if c.Claude.Enabled {
		keys[c.Claude.Key] = "claude"
	}

	for _, session := range c.Sessions {
		if session.Name == "" {
			return fmt.Errorf("session missing name")
		}
		if session.Command == "" {
			return fmt.Errorf("session %q missing command", session.Name)
		}
		if session.Key == "" {
			return fmt.Errorf("session %q missing key", session.Name)
		}

		// Check for duplicate key
		if existing, ok := keys[session.Key]; ok {
			return fmt.Errorf("duplicate key %q used by %q and %q", session.Key, existing, session.Name)
		}
		keys[session.Key] = session.Name
	}

	return nil
}

// AllSessions returns all configured sessions including Claude
func (c *Config) AllSessions() []SessionConfig {
	sessions := []SessionConfig{}

	if c.Claude.Enabled {
		sessions = append(sessions, SessionConfig{
			Name:    "claude",
			Command: c.Claude.Command,
			Key:     c.Claude.Key,
		})
	}

	sessions = append(sessions, c.Sessions...)
	return sessions
}
