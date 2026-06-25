package cli

import (
	"testing"

	"github.com/drift/drift/internal/core"
)

// TC-RM-001: rm removes a tracked file from index and working tree
func TestRm_TrackedFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	output, err := h.RunRm("note.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Removed: note.txt")
	h.AssertContains(output, "Removed 1 file(s)")

	// Working tree file should be deleted.
	if h.FileExists("note.txt") {
		t.Error("note.txt should be removed from working tree")
	}

	// Index should be empty.
	var idx = loadIndex(t, h)
	if len(idx.Entries) != 0 {
		t.Errorf("index should be empty, got %d entries", len(idx.Entries))
	}
}

// TC-RM-002: rm --cached removes from index only, keeps working tree file
func TestRm_CachedKeepsWorktree(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	_, err := h.RunRm("--cached", "note.txt")
	h.AssertNoError(err)

	// Working tree file should still exist.
	if !h.FileExists("note.txt") {
		t.Error("note.txt should still exist in working tree with --cached")
	}

	// Index should be empty.
	var idx = loadIndex(t, h)
	if len(idx.Entries) != 0 {
		t.Errorf("index should be empty, got %d entries", len(idx.Entries))
	}
}

// TC-RM-003: rm rejects untracked files
func TestRm_UntrackedFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")
	h.WriteFile("untracked.txt", "untracked")

	_, err := h.RunRm("untracked.txt")
	h.AssertError(err)
}

// TC-RM-004: rm -r removes a directory recursively
func TestRm_RecursiveDirectory(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("docs/a.txt", "a")
	h.WriteFile("docs/b.txt", "b")
	h.AddAndSave([]string{"docs"}, "v1")

	output, err := h.RunRm("-r", "docs")
	h.AssertNoError(err)
	h.AssertContains(output, "Removed 2 file(s)")

	if h.FileExists("docs/a.txt") || h.FileExists("docs/b.txt") {
		t.Error("docs files should be removed")
	}
}

// TC-RM-005: rm without -r on a directory fails
func TestRm_DirectoryWithoutRecursive(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("docs/a.txt", "a")
	h.AddAndSave([]string{"docs"}, "v1")

	_, err := h.RunRm("docs")
	h.AssertError(err)
}

// TC-RM-006: rm supports glob patterns
func TestRm_GlobPattern(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.tmp", "tmp1")
	h.WriteFile("b.tmp", "tmp2")
	h.WriteFile("keep.txt", "keep")
	h.AddAndSave([]string{"a.tmp", "b.tmp", "keep.txt"}, "v1")

	output, err := h.RunRm("*.tmp")
	h.AssertNoError(err)
	h.AssertContains(output, "Removed 2 file(s)")

	if h.FileExists("a.tmp") || h.FileExists("b.tmp") {
		t.Error("tmp files should be removed")
	}
	if !h.FileExists("keep.txt") {
		t.Error("keep.txt should still exist")
	}
}

// TC-RM-007: rm multiple paths in one call
func TestRm_MultiplePaths(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "a")
	h.WriteFile("b.txt", "b")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v1")

	output, err := h.RunRm("a.txt", "b.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Removed 2 file(s)")
}

// TC-MV-001: mv renames a tracked file
func TestMv_RenameFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("old.txt", "content")
	h.AddAndSave([]string{"old.txt"}, "v1")

	output, err := h.RunMv("old.txt", "new.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Moved: old.txt -> new.txt")
	h.AssertContains(output, "Moved 1 file(s)")

	// old.txt should not exist, new.txt should.
	if h.FileExists("old.txt") {
		t.Error("old.txt should not exist after move")
	}
	if !h.FileExists("new.txt") {
		t.Error("new.txt should exist after move")
	}
	if h.ReadFile("new.txt") != "content" {
		t.Error("new.txt content should match old.txt")
	}

	// Index should have new.txt, not old.txt.
	var idx = loadIndex(t, h)
	if !idx.Has("new.txt") {
		t.Error("index should contain new.txt")
	}
	if idx.Has("old.txt") {
		t.Error("index should not contain old.txt")
	}
}

// TC-MV-002: mv into a directory
func TestMv_IntoDirectory(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.WriteFile("docs/.keep", "")
	h.AddAndSave([]string{"note.txt", "docs/.keep"}, "v1")

	_, err := h.RunMv("note.txt", "docs")
	h.AssertNoError(err)

	if h.FileExists("note.txt") {
		t.Error("note.txt should not exist in root")
	}
	if !h.FileExists("docs/note.txt") {
		t.Error("note.txt should exist in docs/")
	}
}

// TC-MV-003: mv rejects untracked source
func TestMv_UntrackedSource(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("tracked.txt", "content")
	h.AddAndSave([]string{"tracked.txt"}, "v1")
	h.WriteFile("untracked.txt", "untracked")

	_, err := h.RunMv("untracked.txt", "renamed.txt")
	h.AssertError(err)
}

// TC-MV-004: mv multiple files into a directory
func TestMv_MultipleIntoDirectory(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "a")
	h.WriteFile("b.txt", "b")
	h.WriteFile("dest/.keep", "")
	h.AddAndSave([]string{"a.txt", "b.txt", "dest/.keep"}, "v1")

	output, err := h.RunMv("a.txt", "b.txt", "dest")
	h.AssertNoError(err)
	h.AssertContains(output, "Moved 2 file(s)")

	if !h.FileExists("dest/a.txt") || !h.FileExists("dest/b.txt") {
		t.Error("files should be moved into dest/")
	}
}

// TC-MV-005: mv multiple sources to non-directory fails
func TestMv_MultipleToNonDirectory(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "a")
	h.WriteFile("b.txt", "b")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v1")

	_, err := h.RunMv("a.txt", "b.txt", "nonexistent")
	h.AssertError(err)
}

// loadIndex is a small helper to read the current index for assertions.
func loadIndex(t *testing.T, h *TestHelper) *core.Index {
	t.Helper()
	h.SetupSharedState()
	idx := &core.Index{}
	if err := sharedStore.LoadIndex(idx); err != nil {
		t.Fatalf("failed to load index: %v", err)
	}
	return idx
}

// TC-RM-008: rm with -f flag skips confirmation (non-interactive in tests)
func TestRm_WithForce(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	output, err := h.RunRm("-f", "note.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Removed: note.txt")
	h.AssertContains(output, "Removed 1 file(s)")

	if h.FileExists("note.txt") {
		t.Error("note.txt should be removed from working tree")
	}
}

// TC-RM-009: rm without -f still proceeds in non-interactive mode
func TestRm_NonInteractiveAutoProceed(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Without -f, confirmAction auto-proceeds in non-interactive (test) mode.
	output, err := h.RunRm("note.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Removed: note.txt")

	if h.FileExists("note.txt") {
		t.Error("note.txt should be removed from working tree")
	}
}

// TC-RM-010: rm --cached does not require confirmation
func TestRm_CachedNoConfirmation(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// --cached only modifies the index, no confirmation needed.
	_, err := h.RunRm("--cached", "note.txt")
	h.AssertNoError(err)

	if !h.FileExists("note.txt") {
		t.Error("note.txt should still exist with --cached")
	}
}

// TC-MV-006: mv without -f refuses to overwrite existing destination
func TestMv_OverwriteWithoutForce(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("old.txt", "old content")
	h.WriteFile("existing.txt", "existing content")
	h.AddAndSave([]string{"old.txt", "existing.txt"}, "v1")

	_, err := h.RunMv("old.txt", "existing.txt")
	h.AssertError(err)

	// Both files should still exist with original content.
	if !h.FileExists("old.txt") {
		t.Error("old.txt should still exist (move refused)")
	}
	if h.ReadFile("existing.txt") != "existing content" {
		t.Error("existing.txt content should be unchanged")
	}
}

// TC-MV-007: mv with -f overwrites existing destination
func TestMv_OverwriteWithForce(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("old.txt", "old content")
	h.WriteFile("existing.txt", "existing content")
	h.AddAndSave([]string{"old.txt", "existing.txt"}, "v1")

	output, err := h.RunMv("-f", "old.txt", "existing.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Moved: old.txt -> existing.txt")

	// old.txt should be gone, existing.txt should have old.txt's content.
	if h.FileExists("old.txt") {
		t.Error("old.txt should not exist after forced move")
	}
	if !h.FileExists("existing.txt") {
		t.Error("existing.txt should exist after overwrite")
	}
	if h.ReadFile("existing.txt") != "old content" {
		t.Errorf("existing.txt content = %q, want %q",
			h.ReadFile("existing.txt"), "old content")
	}

	// Index should have existing.txt, not old.txt.
	var idx = loadIndex(t, h)
	if idx.Has("old.txt") {
		t.Error("index should not contain old.txt")
	}
	if !idx.Has("existing.txt") {
		t.Error("index should contain existing.txt")
	}
}
