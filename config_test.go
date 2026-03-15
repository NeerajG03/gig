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

func TestLoadConfigInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gig.yaml")
	os.WriteFile(path, []byte("{{{{not yaml"), 0o644)

	_, err := LoadConfig(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoadConfigWithHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gig.yaml")
	yaml := `hooks:
  on_create:
    - command: "echo created {id}"
    - command: "notify {actor}"
      filter:
        assignee: neeraj
  on_close:
    - command: "echo closed"
`
	os.WriteFile(path, []byte(yaml), 0o644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Hooks.OnCreate) != 2 {
		t.Errorf("expected 2 on_create hooks, got %d", len(cfg.Hooks.OnCreate))
	}
	if len(cfg.Hooks.OnClose) != 1 {
		t.Errorf("expected 1 on_close hook, got %d", len(cfg.Hooks.OnClose))
	}
	if cfg.Hooks.OnCreate[1].Filter["assignee"] != "neeraj" {
		t.Errorf("expected filter assignee=neeraj, got %v", cfg.Hooks.OnCreate[1].Filter)
	}
}

func TestSaveAndReloadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "gig.yaml")

	cfg := &Config{
		Prefix:      "saved",
		HashLen:     5,
		DefaultView: "tree",
		ShowAll:     true,
	}

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaded.Prefix != "saved" {
		t.Errorf("prefix = %q, want 'saved'", loaded.Prefix)
	}
	if loaded.HashLen != 5 {
		t.Errorf("hash_len = %d, want 5", loaded.HashLen)
	}
	if loaded.DefaultView != "tree" {
		t.Errorf("default_view = %q, want 'tree'", loaded.DefaultView)
	}
	if !loaded.ShowAll {
		t.Error("show_all should be true")
	}
}

func TestLoadConfigEmptyPrefix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gig.yaml")
	os.WriteFile(path, []byte("prefix: \"\"\n"), 0o644)

	cfg, _ := LoadConfig(path)
	if cfg.Prefix != "gig" {
		t.Errorf("empty prefix should default to 'gig', got %q", cfg.Prefix)
	}
}
