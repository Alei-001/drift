package cli

import (
	"regexp"
	"strings"
	"testing"
)

// TC-HIST-001: Basic history output with commits
func TestHistory_Basic(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "first version")
	id1 := h.AddAndSave([]string{"note.txt"}, "first commit")

	h.WriteFile("note.txt", "second version")
	id2 := h.AddAndSave([]string{"note.txt"}, "second commit")

	output, err := h.RunHistory()
	h.AssertNoError(err)
	h.AssertContains(output, "Version: "+id2)
	h.AssertContains(output, "Version: "+id1)
	h.AssertContains(output, "first commit")
	h.AssertContains(output, "second commit")
	h.AssertContains(output, "Branch:  main")
}

// TC-HIST-002: History oneline mode
func TestHistory_Oneline(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	id1 := h.AddAndSave([]string{"note.txt"}, "first commit")

	h.WriteFile("note.txt", "updated")
	id2 := h.AddAndSave([]string{"note.txt"}, "second commit")

	output, err := h.RunHistoryOneline()
	h.AssertNoError(err)
	h.AssertContains(output, id2+" [main] second commit")
	h.AssertContains(output, id1+" [main] first commit")
}

// TC-HIST-003: History with limit (-n)
func TestHistory_Limit(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	for i := 1; i <= 5; i++ {
		fname := "f" + string(rune('0'+i)) + ".txt"
		h.WriteFile(fname, "content")
		h.AddAndSave([]string{fname}, "")
	}

	output, err := h.RunHistoryLimit(3)
	h.AssertNoError(err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	versionLines := 0
	for _, line := range lines {
		if strings.Contains(line, "Version:") {
			versionLines++
		}
	}
	if versionLines != 3 {
		t.Errorf("expected 3 version lines with -n 3, got %d", versionLines)
	}
}

// TC-HIST-004: History specific branch
func TestHistory_SpecificBranch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("main.txt", "main content")
	h.AddAndSave([]string{"main.txt"}, "main commit")

	_, err := h.RunBranch("feature")
	h.AssertNoError(err)
	_, err = h.RunSwitch("feature")
	h.AssertNoError(err)
	h.WriteFile("feat.txt", "feature content")
	h.AddAndSave([]string{"feat.txt"}, "feature commit")

	output, err := h.RunHistory("main")
	h.AssertNoError(err)
	h.AssertContains(output, "main commit")
	h.AssertNotContains(output, "feature commit")
}

// TC-HIST-005: History with no commits
func TestHistory_NoCommits(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	output, err := h.RunHistory()
	h.AssertNoError(err)
	h.AssertContains(output, "No commits on branch main yet")
}

// TC-HIST-006: History oneline with no message
func TestHistory_OnelineNoMessage(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	id1 := h.AddAndSave([]string{"note.txt"}, "")

	output, err := h.RunHistoryOneline()
	h.AssertNoError(err)
	h.AssertContains(output, id1+" [main] (no message)")
}

// TC-HIST-007: History oneline truncates long messages
func TestHistory_OnelineLongMessage(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	longMsg := strings.Repeat("x", 100)
	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, longMsg)

	output, err := h.RunHistoryOneline()
	h.AssertNoError(err)
	h.AssertContains(output, "...")
}

// TC-HIST-008: History branch inherits parent commits
func TestHistory_BranchInheritsParent(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1 on main")

	_, err := h.RunBranch("child")
	h.AssertNoError(err)

	// child branch inherits main's commit, so history shows it
	output, err := h.RunHistory("child")
	h.AssertNoError(err)
	h.AssertContains(output, "v1 on main")
}

// TC-HIST-009: Filter by branch name (formerly TestList_FilterByBranch)
func TestHistory_FilterByBranch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create v1 on main
	h.WriteFile("main.txt", "main content")
	id1 := h.AddAndSave([]string{"main.txt"}, "v1")

	// Create feature branch with v1
	_, err := h.RunBranch("feature")
	h.AssertNoError(err)
	_, err = h.RunSwitch("feature")
	h.AssertNoError(err)
	h.WriteFile("feat.txt", "feature content")
	h.AddAndSave([]string{"feat.txt"}, "v1")

	// History only main branch
	output, err := h.RunHistory("main")
	h.AssertNoError(err)
	h.AssertContains(output, "Version: "+id1)
	h.AssertContains(output, "Branch:  main")
	h.AssertNotContains(output, "feature")
}

// TC-HIST-010: History nonexistent branch errors (formerly TestList_NonexistentBranch)
func TestHistory_NonexistentBranch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	_, err := h.RunHistory("nonexistent")
	h.AssertError(err)
}

// TC-HIST-011: Deduplicate commits across branches with --all (formerly TestList_DeduplicateAcrossBranches)
func TestHistory_AllDeduplicateAcrossBranches(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("shared.txt", "shared")
	h.AddAndSave([]string{"shared.txt"}, "v1 on main")

	_, err := h.RunBranch("duplicate")
	h.AssertNoError(err)
	_, err = h.RunSwitch("duplicate")
	h.AssertNoError(err)

	// Switch back to main
	_, err = h.RunSwitch("main")
	h.AssertNoError(err)

	// history --all should show v1 only once
	output, err := h.RunHistoryAll()
	h.AssertNoError(err)
	// Count occurrences of "v1 on main" - should appear exactly once
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "v1 on main") {
			count++
		}
	}
	if count > 1 {
		t.Errorf("commit should appear once, appeared %d times", count)
	}
}

// TC-HIST-012: Show version history with --all (formerly TestList_ShowHistory)
func TestHistory_AllShowHistory(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create 3 versions
	h.WriteFile("f1.txt", "v1")
	h.AddAndSave([]string{"f1.txt"}, "v1")

	h.WriteFile("f2.txt", "v2")
	h.AddAndSave([]string{"f2.txt"}, "v2")

	h.WriteFile("f3.txt", "v3")
	h.AddAndSave([]string{"f3.txt"}, "v3")

	output, err := h.RunHistoryAll()
	h.AssertNoError(err)
	h.AssertContains(output, "Version history:")
	h.AssertContains(output, "v3")
	h.AssertContains(output, "v2")
	h.AssertContains(output, "v1")
}

// TC-HIST-013: No versions yet with --all (formerly TestList_NoVersions)
func TestHistory_AllNoVersions(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	output, err := h.RunHistoryAll()
	h.AssertNoError(err)
	h.AssertContains(output, "No versions yet")
}

// TC-HIST-014: Version without message with --all (formerly TestList_NoMessage)
func TestHistory_AllNoMessage(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	id1 := h.AddAndSave([]string{"note.txt"}, "")

	output, err := h.RunHistoryAll()
	h.AssertNoError(err)
	h.AssertContains(output, id1)
	// When message is empty, the line should end with the branch (no trailing message).
	// Format: "  <id>  [main]  <message>"
	// Use regex to extract the message part after the branch.
	re := regexp.MustCompile(regexp.QuoteMeta(id1) + `\s+\[.*?\]\s*(.*)`)
	for _, line := range strings.Split(output, "\n") {
		m := re.FindStringSubmatch(line)
		if m != nil {
			msg := strings.TrimSpace(m[1])
			if msg != "" {
				t.Errorf("expected empty message, got %q in line %q", msg, line)
			}
		}
	}
}

// TC-UNDO-001: undo reverts the most recent operation
func TestUndo_Single(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "content")
	id1 := h.AddAndSave([]string{"a.txt"}, "first save")

	// Verify v1 exists.
	out, _ := h.RunHistory()
	h.AssertContains(out, id1)

	_, err := h.RunUndo(1)
	h.AssertNoError(err)

	// After undo, the save should be reverted — main ref should be empty,
	// so history reports no commits.
	out, _ = h.RunHistory()
	h.AssertContains(out, "No commits on branch main yet")
}

// TC-UNDO-002: undo -n reverts multiple operations
func TestUndo_Multiple(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// 3 saves → 3 operations.
	for i := 1; i <= 3; i++ {
		h.WriteFile("f.txt", "content"+string(rune('0'+i)))
		h.AddAndSave([]string{"f.txt"}, "")
	}

	_, err := h.RunUndo(3)
	h.AssertNoError(err)

	// All 3 saves undone → no commits left.
	out, _ := h.RunHistory()
	h.AssertContains(out, "No commits on branch main yet")
}

// TC-UNDO-003: undo -n stops gracefully when operations run out
func TestUndo_MoreThanAvailable(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "content")
	h.AddAndSave([]string{"a.txt"}, "only save")

	// Ask for 5 undos but only 1 exists.
	_, err := h.RunUndo(5)
	h.AssertNoError(err)

	// Should have undone the 1 available operation.
	out, _ := h.RunHistory()
	h.AssertContains(out, "No commits on branch main yet")
}

// TC-UNDO-004: undo with no operations returns error
func TestUndo_Nothing(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.RunUndo(1)
	if err == nil {
		t.Error("expected error when undoing with no operations, got nil")
	}
}

// TC-UNDO-005: undo -n with invalid count (0) returns error
func TestUndo_InvalidCount(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.RunUndo(0)
	if err == nil {
		t.Error("expected error for undo -n 0, got nil")
	}
}
