package porcelain

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage/backends/memory"
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
	snap1, err := CreateSnapshot(context.Background(), store, dir, "first commit", "test", nil, nil)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	if err := CreateBranch(context.Background(), store, "", "feature"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content v2 modified"), 0644); err != nil {
		t.Fatalf("modify file: %v", err)
	}

	autosaveID, fromBranch, diffCount, err := SwitchBranch(context.Background(), store, dir, "feature", false, false, "test", nil)
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
	snap1, err := CreateSnapshot(context.Background(), store, dir, "first commit", "test", nil, nil)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	autosaveID, fromBranch, diffCount, err := SwitchBranch(context.Background(), store, dir, "experimental", true, false, "test", nil)
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

	_, _, _, err := SwitchBranch(context.Background(), store, dir, "nonexistent", false, false, "test", nil)
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
	snap1, err := CreateSnapshot(context.Background(), store, dir, "first commit", "test", nil, nil)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	if err := CreateBranch(context.Background(), store, "", "feature"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	autosaveID, fromBranch, diffCount, err := SwitchBranch(context.Background(), store, dir, "feature", false, false, "test", nil)
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

// TestSwitchBranch_NoAutosave_CleanWorkspace verifies that noAutosave=true
// with a clean workspace skips the auto-save step entirely: no [auto]
// snapshot is created, autosaveID is empty, and HEAD moves to the target
// branch which inherits the source branch's snapshot as its starting point.
func TestSwitchBranch_NoAutosave_CleanWorkspace(t *testing.T) {
	dir := t.TempDir()
	store := setupSwitchStore()

	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content v1"), 0644)
	snap1, err := CreateSnapshot(context.Background(), store, dir, "first", "test", nil, nil)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if err := CreateBranch(context.Background(), store, "", "feature"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Workspace is clean (no changes since snap1). noAutosave=true should
	// succeed without creating an [auto] snapshot.
	autosaveID, fromBranch, diffCount, err := SwitchBranch(context.Background(), store, dir, "feature", false, true, "test", nil)
	if err != nil {
		t.Fatalf("SwitchBranch noAutosave clean: %v", err)
	}
	if autosaveID != "" {
		t.Errorf("expected empty autosaveID (no autosave), got '%s'", autosaveID)
	}
	if fromBranch != "main" {
		t.Errorf("expected fromBranch 'main', got '%s'", fromBranch)
	}
	if diffCount != 0 {
		t.Errorf("expected diffCount 0, got %d", diffCount)
	}

	headRef, _ := store.GetRef(context.Background(), "HEAD")
	if headRef.SymRef != "heads/feature" {
		t.Errorf("expected HEAD symref 'heads/feature', got '%s'", headRef.SymRef)
	}
	featureRef, _ := store.GetRef(context.Background(), "heads/feature")
	if featureRef.Target != snap1.ID.Hash {
		t.Errorf("expected feature to inherit snap1, got %s", featureRef.Target.String())
	}
}

// TestSwitchBranch_NoAutosave_DirtyWorkspace verifies that noAutosave=true
// with uncommitted changes refuses with ErrUncommittedChanges. HEAD must
// remain on the original branch and the workspace change must be preserved.
func TestSwitchBranch_NoAutosave_DirtyWorkspace(t *testing.T) {
	dir := t.TempDir()
	store := setupSwitchStore()

	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content v1"), 0644)
	snap1, err := CreateSnapshot(context.Background(), store, dir, "first", "test", nil, nil)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if err := CreateBranch(context.Background(), store, "", "feature"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content v2 modified"), 0644)

	_, _, _, err = SwitchBranch(context.Background(), store, dir, "feature", false, true, "test", nil)
	if !errors.Is(err, ErrUncommittedChanges) {
		t.Fatalf("expected ErrUncommittedChanges, got %v", err)
	}

	headRef, _ := store.GetRef(context.Background(), "HEAD")
	if headRef.SymRef != "heads/main" {
		t.Errorf("expected HEAD remain on 'heads/main', got '%s'", headRef.SymRef)
	}
	mainRef, _ := store.GetRef(context.Background(), "heads/main")
	if mainRef.Target != snap1.ID.Hash {
		t.Errorf("expected main remain at snap1, got %s", mainRef.Target.String())
	}
	content, _ := os.ReadFile(filepath.Join(dir, "file1.txt"))
	if string(content) != "content v2 modified" {
		t.Errorf("expected uncommitted change preserved, got %q", string(content))
	}
}
