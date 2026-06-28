package porcelain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitProject(t *testing.T) {
	dir := t.TempDir()

	if err := InitProject(dir); err != nil {
		t.Fatalf("InitProject failed: %v", err)
	}

	// Verify .drift directory exists
	driftPath := filepath.Join(dir, ".drift")
	if _, err := os.Stat(driftPath); err != nil {
		t.Fatalf(".drift directory does not exist: %v", err)
	}

	// Verify subdirectories exist
	for _, sub := range []string{"chunks", "snapshots", "refs", "previews", "refs/heads", "refs/tags"} {
		p := filepath.Join(driftPath, sub)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected directory %s to exist: %v", sub, err)
		}
	}
}

func TestInitProject_AlreadyExists(t *testing.T) {
	dir := t.TempDir()

	if err := InitProject(dir); err != nil {
		t.Fatalf("first InitProject failed: %v", err)
	}

	err := InitProject(dir)
	if err == nil {
		t.Fatal("expected error for already initialized project, got nil")
	}
	if err.Error() != "already a drift repository" {
		t.Errorf("expected 'already a drift repository', got '%s'", err.Error())
	}
}

func TestOpenProject(t *testing.T) {
	dir := t.TempDir()

	if err := InitProject(dir); err != nil {
		t.Fatalf("InitProject failed: %v", err)
	}

	store, config, err := OpenProject(dir)
	if err != nil {
		t.Fatalf("OpenProject failed: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
	if config == nil {
		t.Fatal("expected non-nil config")
	}

	// Verify HEAD reference exists
	ref, err := store.GetRef("HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD failed: %v", err)
	}
	if ref.Type != "HEAD" {
		t.Errorf("expected HEAD type, got %s", ref.Type)
	}
}

func TestOpenProject_NotADriftRepo(t *testing.T) {
	dir := t.TempDir()

	_, _, err := OpenProject(dir)
	if err == nil {
		t.Fatal("expected error for non-drift directory, got nil")
	}
	if err.Error() != "not a drift repository" {
		t.Errorf("expected 'not a drift repository', got '%s'", err.Error())
	}
}
