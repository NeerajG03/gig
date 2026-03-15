package gig

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the gig.yaml configuration file.
type Config struct {
	Prefix      string     `yaml:"prefix"`
	DBPath      string     `yaml:"db_path"`
	HashLen     int        `yaml:"hash_length"`
	SyncRepo    string     `yaml:"sync_repo"`
	DefaultView string     `yaml:"default_view"` // "list" or "tree" (default: "list")
	ShowAll     bool       `yaml:"show_all"`     // if true, include closed tasks by default
	Hooks       HookConfig `yaml:"hooks"`
}

// HookConfig maps event types to lists of hook definitions.
type HookConfig struct {
	OnStatusChange []HookDef `yaml:"on_status_change"`
	OnCreate       []HookDef `yaml:"on_create"`
	OnComment      []HookDef `yaml:"on_comment"`
	OnClose        []HookDef `yaml:"on_close"`
	OnAssign       []HookDef `yaml:"on_assign"`
}

// HookDef defines a single hook command with optional filter.
type HookDef struct {
	Command string            `yaml:"command"`
	Filter  map[string]string `yaml:"filter,omitempty"`
}

// DefaultGigHome returns the default gig home directory (~/.gig).
// Respects GIG_HOME env var.
func DefaultGigHome() string {
	if env := os.Getenv("GIG_HOME"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".gig"
	}
	return filepath.Join(home, ".gig")
}

// DefaultDBPath returns the default database path.
func DefaultDBPath() string {
	return filepath.Join(DefaultGigHome(), "gig.db")
}

// DefaultConfigPath returns the default config file path.
func DefaultConfigPath() string {
	return filepath.Join(DefaultGigHome(), "gig.yaml")
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Prefix:  "gig",
		DBPath:  DefaultDBPath(),
		HashLen: 4,
	}
}

// LoadConfig reads and parses a gig.yaml file.
// Returns DefaultConfig if the file doesn't exist.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			return &cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Apply defaults for empty fields.
	if cfg.Prefix == "" {
		cfg.Prefix = "gig"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = DefaultDBPath()
	}
	if cfg.HashLen < 3 || cfg.HashLen > 8 {
		cfg.HashLen = 4
	}
	if cfg.DefaultView != "" && cfg.DefaultView != "list" && cfg.DefaultView != "tree" {
		cfg.DefaultView = ""
	}

	return &cfg, nil
}

// SaveConfig writes the config to a YAML file.
func SaveConfig(path string, cfg *Config) error {
	if path == "" {
		path = DefaultConfigPath()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}
