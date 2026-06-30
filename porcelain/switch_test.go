package porcelain

import (
	"os"
	"path/filepath"
	"strings"
	"context"
	"testing"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage/memory"
)

func setupSwitchStore() *memory.MemoryStorage {
	store := memory.NewMemoryStorage()
	store.SetRef(context.Background(), "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: core.Hash{},
	})
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	})
	return store
}

func TestSwitchBranch_AutoSaveAndRestore(t *testing.T) {
	dir := t.TempDir()
	store := setupSwitchStore()

	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content v1"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	snap1, err := CreateSnapshot(store, dir, "first commit", "test", nil)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	if err := CreateBranch(store, "feature"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content v2 modified"), 0644); err != nil {
		t.Fatalf("modify file: %v", err)
	}

	autosaveID, fromBranch, diffCount, err := SwitchBranch(store, dir, "feature", false, "test")
	if err != nil {
		t.Fatalf("SwitchBranch failed: %v", err)
	}

	if autosaveID == "" {
		t.Error("expected non-empty autosaveID, got empty")
	}
	if fromBranch != "main" {
		t.Errorf("expected fromBranch 'main', got '%s'", fromBranch)
	}

	headRef, err := store.GetRef(context.Background(), "HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD failed: %v", err)
	}
	if headRef.SymRef != "heads/feature" {
		t.Errorf("expected HEAD symref 'heads/feature', got '%s'", headRef.SymRef)
	}

	content, err := os.ReadFile(filepath.Join(dir, "file1.txt"))
	if err != nil {
		t.Fatalf("read file1.txt: %v", err)
	}
	if string(content) != "content v1" {
		t.Errorf("expected 'content v1', got %q", string(content))
	}

	if diffCount == 0 {
		t.Error("expected diffCount > 0, got 0")
	}

	mainRef, err := store.GetRef(context.Background(), "heads/main")
	if err != nil {
		t.Fatalf("GetRef heads/main failed: %v", err)
	}
	if mainRef.Target == snap1.ID.Hash {
		t.Error("expected main branch to have auto-save snapshot, but it still points to snap1")
	}

	featureRef, err := store.GetRef(context.Background(), "heads/feature")
	if err != nil {
		t.Fatalf("GetRef heads/feature failed: %v", err)
	}
	if featureRef.Target != snap1.ID.Hash {
		t.Error("expected feature branch to point to snap1")
	}
}

func TestSwitchBranch_CreateNew(t *testing.T) {
	dir := t.TempDir()
	store := setupSwitchStore()

	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	snap1, err := CreateSnapshot(store, dir, "first commit", "test", nil)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	autosaveID, fromBranch, diffCount, err := SwitchBranch(store, dir, "experimental", true, "test")
	if err != nil {
		t.Fatalf("SwitchBranch failed: %v", err)
	}

	if autosaveID != "" {
		t.Errorf("expected empty autosaveID (nothing to save), got '%s'", autosaveID)
	}
	if fromBranch != "main" {
		t.Errorf("expected fromBranch 'main', got '%s'", fromBranch)
	}

	headRef, err := store.GetRef(context.Background(), "HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD failed: %v", err)
	}
	if headRef.SymRef != "heads/experimental" {
		t.Errorf("expected HEAD symref 'heads/experimental', got '%s'", headRef.SymRef)
	}

	expRef, err := store.GetRef(context.Background(), "heads/experimental")
	if err != nil {
		t.Fatalf("GetRef heads/experimental failed: %v", err)
	}
	if expRef.Target != snap1.ID.Hash {
		t.Error("expected experimental branch to point to snap1")
	}

	content, err := os.ReadFile(filepath.Join(dir, "file1.txt"))
	if err != nil {
		t.Fatalf("read file1.txt: %v", err)
	}
	if string(content) != "content" {
		t.Errorf("expected 'content', got %q", string(content))
	}

	if diffCount != 0 {
		t.Errorf("expected diffCount 0, got %d", diffCount)
	}
}

func TestSwitchBranch_NotFound(t *testing.T) {
	dir := t.TempDir()
	store := setupSwitchStore()

	_, _, _, err := SwitchBranch(store, dir, "nonexistent", false, "test")
	if err == nil {
		t.Fatal("expected error for non-existent branch, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error containing 'not found', got '%s'", err.Error())
	}
}

func TestSwitchBranch_NoChanges(t *testing.T) {
	dir := t.TempDir()
	store := setupSwitchStore()

	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	snap1, err := CreateSnapshot(store, dir, "first commit", "test", nil)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	if err := CreateBranch(store, "feature"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	autosaveID, fromBranch, diffCount, err := SwitchBranch(store, dir, "feature", false, "test")
	if err != nil {
		t.Fatalf("SwitchBranch failed: %v", err)
	}

	if autosaveID != "" {
		t.Errorf("expected empty autosaveID (nothing to save), got '%s'", autosaveID)
	}
	if fromBranch != "main" {
		t.Errorf("expected fromBranch 'main', got '%s'", fromBranch)
	}

	headRef, err := store.GetRef(context.Background(), "HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD failed: %v", err)
	}
	if headRef.SymRef != "heads/feature" {
		t.Errorf("expected HEAD symref 'heads/feature', got '%s'", headRef.SymRef)
	}

	content, err := os.ReadFile(filepath.Join(dir, "file1.txt"))
	if err != nil {
		t.Fatalf("read file1.txt: %v", err)
	}
	if string(content) != "content" {
		t.Errorf("expected 'content', got %q", string(content))
	}

	if diffCount != 0 {
		t.Errorf("expected diffCount 0, got %d", diffCount)
	}

	featureRef, err := store.GetRef(context.Background(), "heads/feature")
	if err != nil {
		t.Fatalf("GetRef heads/feature failed: %v", err)
	}
	if featureRef.Target != snap1.ID.Hash {
		t.Error("expected feature branch to point to snap1")
	}
}
