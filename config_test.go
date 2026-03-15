package gig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/path")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Prefix != "gig" {
		t.Errorf("prefix = %q, want 'gig'", cfg.Prefix)
	}
	if cfg.HashLen != 4 {
		t.Errorf("hash_len = %d, want 4", cfg.HashLen)
	}
	if cfg.DefaultView != "" {
		t.Errorf("default_view = %q, want empty", cfg.DefaultView)
	}
	if cfg.ShowAll != false {
		t.Error("show_all should default to false")
	}
}

func TestLoadConfigWithViewSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gig.yaml")

	// Valid tree view.
	os.WriteFile(path, []byte("default_view: tree\nshow_all: true\n"), 0o644)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.DefaultView != "tree" {
		t.Errorf("default_view = %q, want 'tree'", cfg.DefaultView)
	}
	if !cfg.ShowAll {
		t.Error("show_all should be true")
	}

	// Valid list view.
	os.WriteFile(path, []byte("default_view: list\n"), 0o644)
	cfg, _ = LoadConfig(path)
	if cfg.DefaultView != "list" {
		t.Errorf("default_view = %q, want 'list'", cfg.DefaultView)
	}

	// Invalid view value gets reset.
	os.WriteFile(path, []byte("default_view: kanban\n"), 0o644)
	cfg, _ = LoadConfig(path)
	if cfg.DefaultView != "" {
		t.Errorf("invalid default_view should be reset to empty, got %q", cfg.DefaultView)
	}
}

func TestLoadConfigHashLenValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gig.yaml")

	// Too small.
	os.WriteFile(path, []byte("hash_length: 1\n"), 0o644)
	cfg, _ := LoadConfig(path)
	if cfg.HashLen != 4 {
		t.Errorf("hash_len = %d, want 4 (default for invalid)", cfg.HashLen)
	}

	// Too large.
	os.WriteFile(path, []byte("hash_length: 20\n"), 0o644)
	cfg, _ = LoadConfig(path)
	if cfg.HashLen != 4 {
		t.Errorf("hash_len = %d, want 4 (default for invalid)", cfg.HashLen)
	}

	// Valid.
	os.WriteFile(path, []byte("hash_length: 6\n"), 0o644)
	cfg, _ = LoadConfig(path)
	if cfg.HashLen != 6 {
		t.Errorf("hash_len = %d, want 6", cfg.HashLen)
	}
}
