package cli

import (
	"testing"
)

// TC-CLEAN-001: clean with no untracked files
func TestClean_NoUntracked(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "a")
	h.AddAndSave([]string{"a.txt"}, "v1")

	output, err := h.RunClean()
	h.AssertNoError(err)
	h.AssertContains(output, "No untracked files to clean")
}

// TC-CLEAN-002: clean deletes untracked files
func TestClean_Basic(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("tracked.txt", "tracked")
	h.AddAndSave([]string{"tracked.txt"}, "v1")
	h.WriteFile("untracked.txt", "untracked")
	h.WriteFile("also_untracked.txt", "also untracked")

	output, err := h.RunClean()
	h.AssertNoError(err)
	h.AssertContains(output, "Deleted: untracked.txt")
	h.AssertContains(output, "Deleted: also_untracked.txt")
	h.AssertContains(output, "Deleted 2 untracked file(s)")

	// Untracked files should be deleted.
	if h.FileExists("untracked.txt") {
		t.Error("untracked.txt should be deleted")
	}
	if h.FileExists("also_untracked.txt") {
		t.Error("also_untracked.txt should be deleted")
	}

	// Tracked file should be preserved.
	if !h.FileExists("tracked.txt") {
		t.Error("tracked.txt should still exist")
	}
}

// TC-CLEAN-003: clean -n is a dry run (lists without deleting)
func TestClean_DryRun(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("tracked.txt", "tracked")
	h.AddAndSave([]string{"tracked.txt"}, "v1")
	h.WriteFile("untracked.txt", "untracked")

	output, err := h.RunClean("-n")
	h.AssertNoError(err)
	h.AssertContains(output, "Would delete 1 untracked file(s):")
	h.AssertContains(output, "untracked.txt")
	h.AssertNotContains(output, "Deleted")

	// File should still exist (dry run).
	if !h.FileExists("untracked.txt") {
		t.Error("untracked.txt should still exist after dry run")
	}
}

// TC-CLEAN-004: clean -f skips confirmation (same behavior in non-interactive tests)
func TestClean_Force(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("tracked.txt", "tracked")
	h.AddAndSave([]string{"tracked.txt"}, "v1")
	h.WriteFile("untracked.txt", "untracked")

	output, err := h.RunClean("-f")
	h.AssertNoError(err)
	h.AssertContains(output, "Deleted: untracked.txt")
	h.AssertContains(output, "Deleted 1 untracked file(s)")

	if h.FileExists("untracked.txt") {
		t.Error("untracked.txt should be deleted")
	}
}

// TC-CLEAN-005: clean -d removes empty directories
func TestClean_Dirs(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("tracked.txt", "tracked")
	h.AddAndSave([]string{"tracked.txt"}, "v1")
	h.WriteFile("untracked_dir/a.txt", "a")
	h.WriteFile("untracked_dir/b.txt", "b")

	output, err := h.RunClean("-d")
	h.AssertNoError(err)
	h.AssertContains(output, "Deleted 2 untracked file(s)")

	// Files should be deleted.
	if h.FileExists("untracked_dir/a.txt") {
		t.Error("untracked_dir/a.txt should be deleted")
	}
	if h.FileExists("untracked_dir/b.txt") {
		t.Error("untracked_dir/b.txt should be deleted")
	}

	// Empty directory should be removed with -d.
	if h.DirExists("untracked_dir") {
		t.Error("untracked_dir should be removed with -d")
	}
}

// TC-CLEAN-006: clean without -d leaves empty directories
func TestClean_WithoutDirs(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("tracked.txt", "tracked")
	h.AddAndSave([]string{"tracked.txt"}, "v1")
	h.WriteFile("untracked_dir/a.txt", "a")

	_, err := h.RunClean()
	h.AssertNoError(err)

	// File should be deleted.
	if h.FileExists("untracked_dir/a.txt") {
		t.Error("untracked_dir/a.txt should be deleted")
	}

	// Empty directory should remain without -d.
	if !h.DirExists("untracked_dir") {
		t.Error("untracked_dir should still exist without -d")
	}
}

// TC-CLEAN-007: clean preserves staged files
func TestClean_PreservesStaged(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("staged.txt", "staged")
	_, err := h.RunAdd("staged.txt")
	h.AssertNoError(err)
	h.WriteFile("untracked.txt", "untracked")

	_, err = h.RunClean()
	h.AssertNoError(err)

	// Staged file should be preserved.
	if !h.FileExists("staged.txt") {
		t.Error("staged.txt should still exist (it is staged)")
	}

	// Untracked file should be deleted.
	if h.FileExists("untracked.txt") {
		t.Error("untracked.txt should be deleted")
	}
}

// TC-CLEAN-008: clean -n -d previews directory removal
func TestClean_DryRunWithDirs(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("tracked.txt", "tracked")
	h.AddAndSave([]string{"tracked.txt"}, "v1")
	h.WriteFile("untracked_dir/a.txt", "a")

	output, err := h.RunClean("-n", "-d")
	h.AssertNoError(err)
	h.AssertContains(output, "Would delete 1 untracked file(s):")
	h.AssertContains(output, "untracked_dir/a.txt")

	// File and directory should still exist (dry run).
	if !h.FileExists("untracked_dir/a.txt") {
		t.Error("untracked_dir/a.txt should still exist after dry run")
	}
	if !h.DirExists("untracked_dir") {
		t.Error("untracked_dir should still exist after dry run")
	}
}
