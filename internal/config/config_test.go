package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultConfig verifies that DefaultConfig sets the default branch.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Core.DefaultBranch != "main" {
		t.Fatalf("expected default branch 'main', got %q", cfg.Core.DefaultBranch)
	}
}

// TestLoadConfig_MissingFile verifies that a missing config file returns DefaultConfig without error.
func TestLoadConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Core.DefaultBranch != "main" {
		t.Fatalf("expected default branch 'main', got %q", cfg.Core.DefaultBranch)
	}
}

// TestSaveThenLoad_RoundTrip verifies that a saved config can be loaded back.
func TestSaveThenLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		User: UserConfig{Name: "alice", Email: "alice@example.com"},
		Core: CoreConfig{DefaultBranch: "develop"},
	}
	if err := SaveConfig(dir, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.User.Name != "alice" {
		t.Fatalf("expected name alice, got %q", loaded.User.Name)
	}
	if loaded.User.Email != "alice@example.com" {
		t.Fatalf("expected email alice@example.com, got %q", loaded.User.Email)
	}
	if loaded.Core.DefaultBranch != "develop" {
		t.Fatalf("expected default branch develop, got %q", loaded.Core.DefaultBranch)
	}
}

// TestLoadConfig_InvalidJSON verifies that a corrupted config file returns an error.
func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// TestLoadConfig_PartialJSON verifies that a partial JSON config falls back to defaults for missing fields.
func TestLoadConfig_PartialJSON(t *testing.T) {
	dir := t.TempDir()
	// Only set user.name; default_branch should fall back to "main".
	content := `{"user": {"name": "bob"}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.User.Name != "bob" {
		t.Fatalf("expected name bob, got %q", cfg.User.Name)
	}
	if cfg.Core.DefaultBranch != "main" {
		t.Fatalf("expected default branch main, got %q", cfg.Core.DefaultBranch)
	}
}

// TestSaveConfig_CreatesFile verifies that SaveConfig writes a file to the drift directory.
func TestSaveConfig_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := SaveConfig(dir, DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Fatalf("expected config.json to exist: %v", err)
	}
}
