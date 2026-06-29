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
