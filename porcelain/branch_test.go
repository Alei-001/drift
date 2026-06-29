package porcelain

import (
	"strings"
	"testing"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage/memory"
)

func setupBranchStore() *memory.MemoryStorage {
	store := memory.NewMemoryStorage()
	store.SetRef("heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: core.Hash{},
	})
	store.SetRef("HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	})
	return store
}

func TestCreateBranch_FromHead(t *testing.T) {
	store := setupBranchStore()

	targetHash := core.Hash{0x12, 0xab}
	store.SetRef("heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: targetHash,
	})

	err := CreateBranch(store, "feature")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	ref, err := store.GetRef("heads/feature")
	if err != nil {
		t.Fatalf("GetRef heads/feature failed: %v", err)
	}
	if ref.Target != targetHash {
		t.Errorf("expected target %x, got %x", targetHash, ref.Target)
	}
}

func TestCreateBranch_AlreadyExists(t *testing.T) {
	store := setupBranchStore()

	err := CreateBranch(store, "feature")
	if err != nil {
		t.Fatalf("first CreateBranch failed: %v", err)
	}

	err = CreateBranch(store, "feature")
	if err == nil {
		t.Fatal("expected error for duplicate branch, got nil")
	}
}

func TestCreateBranch_InvalidName(t *testing.T) {
	store := setupBranchStore()

	if err := CreateBranch(store, ""); err == nil {
		t.Error("expected error for empty name, got nil")
	}

	if err := CreateBranch(store, "foo..bar"); err == nil {
		t.Error("expected error for name with '..', got nil")
	}

	if err := CreateBranch(store, "foo/bar"); err == nil {
		t.Error("expected error for name with path separator, got nil")
	}
}

func TestListBranches(t *testing.T) {
	store := setupBranchStore()

	if err := CreateBranch(store, "feature"); err != nil {
		t.Fatalf("CreateBranch feature failed: %v", err)
	}
	if err := CreateBranch(store, "dev"); err != nil {
		t.Fatalf("CreateBranch dev failed: %v", err)
	}

	branches, current, err := ListBranches(store)
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}

	if len(branches) != 3 {
		t.Errorf("expected 3 branches, got %d", len(branches))
	}

	if current != "main" {
		t.Errorf("expected current branch 'main', got '%s'", current)
	}
}

func TestDeleteBranch_Success(t *testing.T) {
	store := setupBranchStore()
	if err := CreateBranch(store, "feature"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	err := DeleteBranch(store, "feature")
	if err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	if _, err := store.GetRef("heads/feature"); err == nil {
		t.Error("expected heads/feature to be deleted, but it still exists")
	}
}

func TestDeleteBranch_NotFound(t *testing.T) {
	store := setupBranchStore()

	err := DeleteBranch(store, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent branch, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error containing 'not found', got '%s'", err.Error())
	}
}

func TestDeleteBranch_CurrentBranch(t *testing.T) {
	store := setupBranchStore()
	CreateBranch(store, "feature")
	store.SetRef("HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/feature",
	})

	err := DeleteBranch(store, "feature")
	if err == nil {
		t.Fatal("expected error for deleting current branch, got nil")
	}
	if !strings.Contains(err.Error(), "cannot delete the current branch") {
		t.Errorf("expected error containing 'cannot delete the current branch', got '%s'", err.Error())
	}
}

func TestDeleteBranch_MainProtected(t *testing.T) {
	store := setupBranchStore()
	CreateBranch(store, "other")
	store.SetRef("HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/other",
	})

	err := DeleteBranch(store, "main")
	if err == nil {
		t.Fatal("expected error for deleting main, got nil")
	}
	if !strings.Contains(err.Error(), "cannot delete 'main'") {
		t.Errorf("expected error containing \"cannot delete 'main'\", got '%s'", err.Error())
	}
}

func TestDeleteBranch_EmptyName(t *testing.T) {
	store := setupBranchStore()

	err := DeleteBranch(store, "")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestRenameBranch_NonCurrent(t *testing.T) {
	store := setupBranchStore()
	if err := CreateBranch(store, "feature"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	err := RenameBranch(store, "feature", "dev")
	if err != nil {
		t.Fatalf("RenameBranch failed: %v", err)
	}

	if _, err := store.GetRef("heads/feature"); err == nil {
		t.Error("expected heads/feature to be removed, but it still exists")
	}
	newRef, err := store.GetRef("heads/dev")
	if err != nil {
		t.Fatalf("GetRef heads/dev failed: %v", err)
	}
	if newRef.Name != "heads/dev" {
		t.Errorf("expected ref name 'heads/dev', got '%s'", newRef.Name)
	}

	// HEAD should be unchanged (still points to main).
	headRef, _ := store.GetRef("HEAD")
	if headRef.SymRef != "heads/main" {
		t.Errorf("expected HEAD still at 'heads/main', got '%s'", headRef.SymRef)
	}
}

func TestRenameBranch_CurrentBranch_UpdatesHEAD(t *testing.T) {
	store := setupBranchStore()
	CreateBranch(store, "feature")
	store.SetRef("HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/feature",
	})

	err := RenameBranch(store, "feature", "dev")
	if err != nil {
		t.Fatalf("RenameBranch failed: %v", err)
	}

	if _, err := store.GetRef("heads/feature"); err == nil {
		t.Error("expected heads/feature to be removed, but it still exists")
	}
	if _, err := store.GetRef("heads/dev"); err != nil {
		t.Errorf("expected heads/dev to exist, got error: %v", err)
	}
	headRef, _ := store.GetRef("HEAD")
	if headRef.SymRef != "heads/dev" {
		t.Errorf("expected HEAD SymRef 'heads/dev', got '%s'", headRef.SymRef)
	}
}

func TestRenameBranch_TargetExists(t *testing.T) {
	store := setupBranchStore()
	CreateBranch(store, "feature")
	CreateBranch(store, "dev")

	err := RenameBranch(store, "feature", "dev")
	if err == nil {
		t.Fatal("expected error for existing target name, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected error containing 'already exists', got '%s'", err.Error())
	}

	// Both original branches should still be intact.
	if _, err := store.GetRef("heads/feature"); err != nil {
		t.Error("heads/feature should still exist after failed rename")
	}
}

func TestRenameBranch_NotFound(t *testing.T) {
	store := setupBranchStore()

	err := RenameBranch(store, "nonexistent", "dev")
	if err == nil {
		t.Fatal("expected error for non-existent source branch, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error containing 'not found', got '%s'", err.Error())
	}
}

func TestRenameBranch_MainProtected(t *testing.T) {
	store := setupBranchStore()
	CreateBranch(store, "other")
	store.SetRef("HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/other",
	})

	err := RenameBranch(store, "main", "trunk")
	if err == nil {
		t.Fatal("expected error for renaming main, got nil")
	}
	if !strings.Contains(err.Error(), "cannot rename 'main'") {
		t.Errorf("expected error containing \"cannot rename 'main'\", got '%s'", err.Error())
	}
}

func TestRenameBranch_SameName_NoOp(t *testing.T) {
	store := setupBranchStore()
	CreateBranch(store, "feature")

	err := RenameBranch(store, "feature", "feature")
	if err != nil {
		t.Fatalf("rename to same name should be a no-op, got: %v", err)
	}
	if _, err := store.GetRef("heads/feature"); err != nil {
		t.Error("heads/feature should still exist after same-name rename")
	}
}

func TestRenameBranch_SameName_NonExistent(t *testing.T) {
	store := setupBranchStore()

	// Renaming a non-existent branch to itself must NOT silently succeed;
	// the source branch existence check runs before the same-name no-op.
	err := RenameBranch(store, "ghost", "ghost")
	if err == nil {
		t.Fatal("expected error for non-existent branch, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error containing 'not found', got '%s'", err.Error())
	}
}

func TestRenameBranch_InvalidNewName(t *testing.T) {
	store := setupBranchStore()
	CreateBranch(store, "feature")

	if err := RenameBranch(store, "feature", "foo..bar"); err == nil {
		t.Error("expected error for new name with '..', got nil")
	}
	if err := RenameBranch(store, "feature", "foo/bar"); err == nil {
		t.Error("expected error for new name with path separator, got nil")
	}
}

func TestRenameBranch_PreservesTarget(t *testing.T) {
	store := setupBranchStore()
	targetHash := core.Hash{0x12, 0xab}
	store.SetRef("heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: targetHash,
	})
	CreateBranch(store, "feature")
	// Point feature at a distinct target.
	store.SetRef("heads/feature", &core.Reference{
		Name:   "heads/feature",
		Type:   core.RefTypeBranch,
		Target: targetHash,
	})

	if err := RenameBranch(store, "feature", "dev"); err != nil {
		t.Fatalf("RenameBranch failed: %v", err)
	}
	devRef, _ := store.GetRef("heads/dev")
	if devRef.Target != targetHash {
		t.Errorf("expected target %x preserved, got %x", targetHash, devRef.Target)
	}
}

func TestRenameBranch_EmptyNames(t *testing.T) {
	store := setupBranchStore()
	CreateBranch(store, "feature")

	if err := RenameBranch(store, "", "dev"); err == nil {
		t.Error("expected error for empty old name, got nil")
	}
	if err := RenameBranch(store, "feature", ""); err == nil {
		t.Error("expected error for empty new name, got nil")
	}
}
