package porcelain

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/your-org/drift/internal/core"
)

// TestDetectChanges_NoIndexAllAdded verifies that when the index is empty,
// every workspace file (except .driftignore and .drift/) is reported as added.
func TestDetectChanges_NoIndexAllAdded(t *testing.T) {
	store, dir := setupLockedProject(t)

	// Empty the index so nothing is "known" yet.
	store.SetIndex(context.Background(), &core.Index{
		Entries:   []core.IndexEntry{},
		UpdatedAt: time.Now().Unix(),
	})

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}

	summary, err := DetectChanges(context.Background(), store, dir, nil)
	if err != nil {
		t.Fatalf("DetectChanges failed: %v", err)
	}
	if !contains(summary.Added, "a.txt") {
		t.Errorf("expected Added to contain a.txt, got %v", summary.Added)
	}
	if !contains(summary.Added, "b.txt") {
		t.Errorf("expected Added to contain b.txt, got %v", summary.Added)
	}
	if len(summary.Modified) != 0 {
		t.Errorf("expected 0 modified, got %d: %v", len(summary.Modified), summary.Modified)
	}
	if len(summary.Deleted) != 0 {
		t.Errorf("expected 0 deleted, got %d: %v", len(summary.Deleted), summary.Deleted)
	}
}

// TestDetectChanges_NoChangesAfterSnapshot verifies that right after a
// successful snapshot, DetectChanges reports no added/modified/deleted files.
func TestDetectChanges_NoChangesAfterSnapshot(t *testing.T) {
	store, dir := setupLockedProject(t)

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := CreateSnapshot(context.Background(), store, dir, "init", "test", nil, nil); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	summary, err := DetectChanges(context.Background(), store, dir, nil)
	if err != nil {
		t.Fatalf("DetectChanges failed: %v", err)
	}
	if len(summary.Added) != 0 || len(summary.Modified) != 0 || len(summary.Deleted) != 0 {
		t.Errorf("expected no changes, got added=%v modified=%v deleted=%v",
			summary.Added, summary.Modified, summary.Deleted)
	}
}

// TestDetectChanges_AddedModifiedDeleted verifies the full mix of changes:
// a new file, a modified file (size change), and a deleted file are all
// detected.
func TestDetectChanges_AddedModifiedDeleted(t *testing.T) {
	store, dir := setupLockedProject(t)

	// Initial snapshot with a.txt and b.txt.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb"), 0644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}
	if _, err := CreateSnapshot(context.Background(), store, dir, "init", "test", nil, nil); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	// Modify a.txt (different size so it's detected), delete b.txt, add c.txt.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa-modified"), 0644); err != nil {
		t.Fatalf("modify a.txt: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "b.txt")); err != nil {
		t.Fatalf("remove b.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("ccc"), 0644); err != nil {
		t.Fatalf("write c.txt: %v", err)
	}

	summary, err := DetectChanges(context.Background(), store, dir, nil)
	if err != nil {
		t.Fatalf("DetectChanges failed: %v", err)
	}
	if !sliceEq(summary.Modified, []string{"a.txt"}) {
		t.Errorf("expected modified=[a.txt], got %v", summary.Modified)
	}
	if !sliceEq(summary.Deleted, []string{"b.txt"}) {
		t.Errorf("expected deleted=[b.txt], got %v", summary.Deleted)
	}
	if !contains(summary.Added, "c.txt") {
		t.Errorf("expected added to contain c.txt, got %v", summary.Added)
	}
}

// TestDetectChanges_SortedOutput verifies that the returned slices are sorted
// alphabetically. Callers rely on stable ordering for diff output.
func TestDetectChanges_SortedOutput(t *testing.T) {
	store, dir := setupLockedProject(t)
	store.SetIndex(context.Background(), &core.Index{
		Entries:   []core.IndexEntry{},
		UpdatedAt: time.Now().Unix(),
	})

	// Create files in non-sorted order.
	for _, name := range []string{"z.txt", "m.txt", "a.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	summary, err := DetectChanges(context.Background(), store, dir, nil)
	if err != nil {
		t.Fatalf("DetectChanges failed: %v", err)
	}
	if !sort.StringsAreSorted(summary.Added) {
		t.Errorf("expected Added sorted, got %v", summary.Added)
	}
}

// --- helpers ---

// sliceEq compares two string slices for equality (order-sensitive).
func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// contains reports whether s contains v.
func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
