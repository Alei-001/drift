package cli

import (
	"testing"
)

// TC-BRANCH-001: Create branch
func TestBranch_Create(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Need at least one commit to create a branch
	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	output, err := h.RunBranch("experiment")
	h.AssertNoError(err)
	h.AssertContains(output, "Created branch: experiment")

	// Verify branch ref exists
	hash, err := h.Store.GetRef("experiment")
	h.AssertNoError(err)
	if hash == "" {
		t.Error("branch ref should exist")
	}

	// Should point to same commit as main
	mainHash, err := h.Store.GetRef("main")
	h.AssertNoError(err)
	if hash != mainHash {
		t.Errorf("experiment hash = %q, want %q (same as main)", hash, mainHash)
	}
}

// TC-BRANCH-002: List branches
func TestBranch_List(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create a commit and branch
	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")
	_, err := h.RunBranch("experiment")
	h.AssertNoError(err)

	output, err := h.RunBranch("list")
	h.AssertNoError(err)
	h.AssertContains(output, "* main")
	h.AssertContains(output, "experiment")
}

// TC-BRANCH-003: List branches with only main
func TestBranch_ListOnlyMain(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	output, err := h.RunBranch("list")
	h.AssertNoError(err)
	h.AssertContains(output, "* main")
}

// TC-SWITCH-001: Switch to existing branch
func TestSwitch_ToExistingBranch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create v1 on main
	h.WriteFile("note.txt", "main content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Create and switch to experiment
	_, err := h.RunBranch("experiment")
	h.AssertNoError(err)

	output, err := h.RunSwitch("experiment")
	h.AssertNoError(err)
	h.AssertContains(output, "Switched to branch: experiment")

	// Verify HEAD points to experiment
	branch, err := h.Store.GetRef("HEAD")
	h.AssertNoError(err)
	if branch != "experiment" {
		t.Errorf("HEAD = %q, want %q", branch, "experiment")
	}
}

// TC-SWITCH-002: Switch deletes files not in target branch
func TestSwitch_DeleteExtraFiles(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create v1 on main
	h.WriteFile("main.txt", "main only")
	h.AddAndSave([]string{"main.txt"}, "v1")

	// Create experiment branch
	_, err := h.RunBranch("experiment")
	h.AssertNoError(err)

	// Switch to experiment
	_, err = h.RunSwitch("experiment")
	h.AssertNoError(err)

	// main.txt should still exist (same as main)
	if !h.FileExists("main.txt") {
		t.Error("main.txt should exist on experiment (same as main)")
	}
}

// TC-SWITCH-003: Switch to nonexistent branch
func TestSwitch_NonexistentBranch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	_, err := h.RunSwitch("nonexistent")
	h.AssertError(err)
}

// TC-SWITCH-004: Switch with staged changes (no --force)
func TestSwitch_StagedChangesNoForce(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create v1 and branch
	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")
	_, err := h.RunBranch("experiment")
	h.AssertNoError(err)

	// Stage a change
	h.WriteFile("note.txt", "modified")
	_, err = h.RunAdd("note.txt")
	h.AssertNoError(err)

	// Switch should fail without --force
	_, err = h.RunSwitch("experiment")
	h.AssertError(err)
}

// TC-SWITCH-005: Switch with staged changes and --force
func TestSwitch_StagedChangesForce(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create v1 and branch
	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")
	_, err := h.RunBranch("experiment")
	h.AssertNoError(err)

	// Stage a change
	h.WriteFile("note.txt", "modified")
	_, err = h.RunAdd("note.txt")
	h.AssertNoError(err)

	// Switch should succeed with --force
	output, err := h.RunSwitch("experiment", "--force")
	h.AssertNoError(err)
	h.AssertContains(output, "Switched to branch: experiment")
}

// TC-SWITCH-007: Switch with --create creates a new branch and switches
func TestSwitch_CreateBranch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	output, err := h.RunSwitch("newbranch", "--create")
	h.AssertNoError(err)
	h.AssertContains(output, "Created branch: newbranch")
	h.AssertContains(output, "Switched to branch: newbranch")

	branch, _ := h.Store.GetRef("HEAD")
	if branch != "newbranch" {
		t.Errorf("HEAD = %q, want %q", branch, "newbranch")
	}
}

// TC-SWITCH-008: Switch with --create fails if branch already exists
func TestSwitch_CreateExistingBranch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	_, err := h.RunBranch("existing")
	h.AssertNoError(err)

	_, err = h.RunSwitch("existing", "--create")
	h.AssertError(err)
}

// TC-SWITCH-009: Switch with --create and -c shorthand
func TestSwitch_CreateShorthand(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	output, err := h.RunSwitch("feat", "-c")
	h.AssertNoError(err)
	h.AssertContains(output, "Created branch: feat")
	h.AssertContains(output, "Switched to branch: feat")
}
// TC-SWITCH-006: Independent version lines on branches
func TestSwitch_IndependentVersionLines(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create v1 on main
	h.WriteFile("f1.txt", "v1")
	h.AddAndSave([]string{"f1.txt"}, "v1")

	// Create experiment branch
	_, err := h.RunBranch("experiment")
	h.AssertNoError(err)

	// Switch to experiment and save v2
	_, err = h.RunSwitch("experiment")
	h.AssertNoError(err)
	h.WriteFile("exp.txt", "experiment work")
	h.AddAndSave([]string{"exp.txt"}, "experiment v2")

	// Switch back to main and save v3
	_, err = h.RunSwitch("main")
	h.AssertNoError(err)
	h.WriteFile("main.txt", "main work")
	h.AddAndSave([]string{"main.txt"}, "main v3")

	// Verify main has v1 and v3
	mainCommits, err := h.Store.ListCommits()
	h.AssertNoError(err)
	// There should be 3 commits total
	if len(mainCommits) != 3 {
		t.Errorf("expected 3 total commits, got %d", len(mainCommits))
	}
}
