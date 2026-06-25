package cli

import (
	"path/filepath"
	"testing"
)

// wipFilePath returns the path to the WIP file for a branch, relative to
// the project root.
func wipFilePath(branch string) string {
	return filepath.ToSlash(filepath.Join(".drift", "wip", branch+".json"))
}

// --- wip list ---

// TC-WIP-001: 'wip list' with no saved WIP prints a friendly message.
func TestWipList_Empty(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	output, err := h.RunWipList()
	h.AssertNoError(err)
	h.AssertContains(output, "No saved work-in-progress")
}

// TC-WIP-002: 'wip list' shows branches with saved WIP.
func TestWipList_WithWIP(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create v1 on main and a branch to switch to.
	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")
	_, err := h.RunBranch("experiment")
	h.AssertNoError(err)

	// Stage a change on main, then switch — this auto-saves WIP for main.
	h.WriteFile("note.txt", "modified")
	_, err = h.RunAdd("note.txt")
	h.AssertNoError(err)
	_, err = h.RunSwitch("experiment")
	h.AssertNoError(err)

	// WIP for main should now appear in the list.
	output, err := h.RunWipList()
	h.AssertNoError(err)
	h.AssertContains(output, "main")
	h.AssertContains(output, "file(s)")
}

// --- wip save ---

// TC-WIP-003: 'wip save' with staged changes saves WIP for the current branch.
func TestWipSave_WithStagedChanges(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Stage a modification.
	h.WriteFile("note.txt", "modified")
	_, err := h.RunAdd("note.txt")
	h.AssertNoError(err)

	// Manually save WIP.
	output, err := h.RunWipSave()
	h.AssertNoError(err)
	h.AssertContains(output, "Saved work-in-progress for branch main")

	// WIP file should exist for main.
	if !h.FileExists(wipFilePath("main")) {
		t.Error("expected WIP file for main to exist after wip save")
	}
}

// TC-WIP-004: 'wip save' clears the index after saving.
func TestWipSave_ClearsIndex(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Stage a modification.
	h.WriteFile("note.txt", "modified")
	_, err := h.RunAdd("note.txt")
	h.AssertNoError(err)

	// Save WIP — this should clear the index.
	_, err = h.RunWipSave()
	h.AssertNoError(err)

	// Status should show no staged changes (the file is still modified in
	// the working tree, but the index is cleared).
	output, err := h.RunStatus()
	h.AssertNoError(err)
	h.AssertNotContains(output, "Staged changes:")
}

// TC-WIP-005: 'wip save' with no changes is a no-op (no WIP file created).
func TestWipSave_NoChanges(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// No pending changes — wip save should succeed but not create a WIP file.
	output, err := h.RunWipSave()
	h.AssertNoError(err)
	h.AssertContains(output, "Saved work-in-progress for branch main")

	if h.FileExists(wipFilePath("main")) {
		t.Error("did not expect WIP file for main when there are no changes")
	}
}

// TC-WIP-006: 'wip save' captures unstaged worktree modifications.
func TestWipSave_CapturesUnstagedChanges(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Modify the file without staging.
	h.WriteFile("note.txt", "unstaged modification")

	// wip save should capture the unstaged change.
	_, err := h.RunWipSave()
	h.AssertNoError(err)

	if !h.FileExists(wipFilePath("main")) {
		t.Error("expected WIP file for main to exist after wip save with unstaged changes")
	}
}

// --- wip restore ---

// TC-WIP-007: 'wip restore' with no WIP prints a friendly message.
func TestWipRestore_NoWIP(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	output, err := h.RunWipRestore()
	h.AssertNoError(err)
	h.AssertContains(output, "No saved work-in-progress for branch main")
}

// TC-WIP-008: 'wip restore' restores files saved via switch auto-save.
func TestWipRestore_AfterSwitch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")
	_, err := h.RunBranch("experiment")
	h.AssertNoError(err)

	// Stage a change on main, then switch — auto-saves WIP for main.
	h.WriteFile("note.txt", "modified-on-main")
	_, err = h.RunAdd("note.txt")
	h.AssertNoError(err)
	_, err = h.RunSwitch("experiment")
	h.AssertNoError(err)

	// Switch back to main and restore the WIP.
	_, err = h.RunSwitch("main")
	h.AssertNoError(err)
	output, err := h.RunWipRestore()
	h.AssertNoError(err)
	h.AssertContains(output, "Restored")
	h.AssertContains(output, "file(s) from work-in-progress for main")

	// WIP file should be deleted after restore.
	if h.FileExists(wipFilePath("main")) {
		t.Error("WIP file for main should be deleted after restore")
	}
}

// TC-WIP-009: 'wip restore' for a specific branch.
func TestWipRestore_SpecificBranch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")
	_, err := h.RunBranch("experiment")
	h.AssertNoError(err)

	// Stage a change on main, then switch — auto-saves WIP for main.
	h.WriteFile("note.txt", "modified-on-main")
	_, err = h.RunAdd("note.txt")
	h.AssertNoError(err)
	_, err = h.RunSwitch("experiment")
	h.AssertNoError(err)

	// Restore WIP for main while on experiment.
	output, err := h.RunWipRestore("main")
	h.AssertNoError(err)
	h.AssertContains(output, "Restored")
	h.AssertContains(output, "for main")

	if h.FileExists(wipFilePath("main")) {
		t.Error("WIP file for main should be deleted after restore")
	}
}

// TC-WIP-010: 'wip restore' round-trip with manual 'wip save'.
func TestWipRestore_AfterManualSave(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Stage a change and manually save WIP.
	h.WriteFile("note.txt", "modified")
	_, err := h.RunAdd("note.txt")
	h.AssertNoError(err)
	_, err = h.RunWipSave()
	h.AssertNoError(err)

	// Restore the WIP.
	output, err := h.RunWipRestore()
	h.AssertNoError(err)
	h.AssertContains(output, "Restored")
	h.AssertContains(output, "file(s) from work-in-progress for main")

	if h.FileExists(wipFilePath("main")) {
		t.Error("WIP file for main should be deleted after restore")
	}
}

// --- wip drop ---

// TC-WIP-011: 'wip drop' with no WIP prints a friendly message.
func TestWipDrop_NoWIP(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	output, err := h.RunWipDrop()
	h.AssertNoError(err)
	h.AssertContains(output, "No saved work-in-progress for branch main")
}

// TC-WIP-012: 'wip drop --force' deletes the WIP file.
func TestWipDrop_Force(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")
	_, err := h.RunBranch("experiment")
	h.AssertNoError(err)

	// Stage a change on main, then switch — auto-saves WIP for main.
	h.WriteFile("note.txt", "modified")
	_, err = h.RunAdd("note.txt")
	h.AssertNoError(err)
	_, err = h.RunSwitch("experiment")
	h.AssertNoError(err)

	// Switch back to main so 'wip drop' targets main.
	_, err = h.RunSwitch("main")
	h.AssertNoError(err)
	if !h.FileExists(wipFilePath("main")) {
		t.Fatal("expected WIP file for main to exist before drop")
	}

	// Drop with --force (skips confirmation, which auto-proceeds in tests anyway).
	output, err := h.RunWipDrop("--force")
	h.AssertNoError(err)
	h.AssertContains(output, "Dropped work-in-progress for branch main")

	if h.FileExists(wipFilePath("main")) {
		t.Error("WIP file for main should be deleted after drop")
	}
}

// TC-WIP-013: 'wip drop' for a specific branch.
func TestWipDrop_SpecificBranch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")
	_, err := h.RunBranch("experiment")
	h.AssertNoError(err)

	// Stage a change on main, then switch — auto-saves WIP for main.
	h.WriteFile("note.txt", "modified")
	_, err = h.RunAdd("note.txt")
	h.AssertNoError(err)
	_, err = h.RunSwitch("experiment")
	h.AssertNoError(err)
	if !h.FileExists(wipFilePath("main")) {
		t.Fatal("expected WIP file for main to exist before drop")
	}

	// Drop WIP for main while on experiment.
	output, err := h.RunWipDrop("main", "--force")
	h.AssertNoError(err)
	h.AssertContains(output, "Dropped work-in-progress for branch main")

	if h.FileExists(wipFilePath("main")) {
		t.Error("WIP file for main should be deleted after drop")
	}
}
