package cli

import (
	"strings"
	"testing"
)

// TC-LOG-001: Basic log output with commits
func TestLog_Basic(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "first version")
	h.AddAndSave([]string{"note.txt"}, "first commit")

	h.WriteFile("note.txt", "second version")
	h.AddAndSave([]string{"note.txt"}, "second commit")

	output, err := h.RunLog()
	h.AssertNoError(err)
	h.AssertContains(output, "Version: v2")
	h.AssertContains(output, "Version: v1")
	h.AssertContains(output, "first commit")
	h.AssertContains(output, "second commit")
	h.AssertContains(output, "Branch:  main")
}

// TC-LOG-002: Log oneline mode
func TestLog_Oneline(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "first commit")

	h.WriteFile("note.txt", "updated")
	h.AddAndSave([]string{"note.txt"}, "second commit")

	output, err := h.RunLogOneline()
	h.AssertNoError(err)
	h.AssertContains(output, "v2 [main] second commit")
	h.AssertContains(output, "v1 [main] first commit")
}

// TC-LOG-003: Log with limit (-n)
func TestLog_Limit(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	for i := 1; i <= 5; i++ {
		fname := "f" + string(rune('0'+i)) + ".txt"
		h.WriteFile(fname, "content")
		h.AddAndSave([]string{fname}, "")
	}

	output, err := h.RunLogLimit(3)
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

// TC-LOG-004: Log specific branch
func TestLog_SpecificBranch(t *testing.T) {
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

	output, err := h.RunLog("main")
	h.AssertNoError(err)
	h.AssertContains(output, "main commit")
	h.AssertNotContains(output, "feature commit")
}

// TC-LOG-005: Log with no commits
func TestLog_NoCommits(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	output, err := h.RunLog()
	h.AssertNoError(err)
	h.AssertContains(output, "No commits on branch main yet")
}

// TC-LOG-006: Log oneline with no message
func TestLog_OnelineNoMessage(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "")

	output, err := h.RunLogOneline()
	h.AssertNoError(err)
	h.AssertContains(output, "v1 [main] (no message)")
}

// TC-LOG-007: Log oneline truncates long messages
func TestLog_OnelineLongMessage(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	longMsg := strings.Repeat("x", 100)
	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, longMsg)

	output, err := h.RunLogOneline()
	h.AssertNoError(err)
	h.AssertContains(output, "...")
}

// TC-LOG-008: Log branch inherits parent commits
func TestLog_BranchInheritsParent(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1 on main")

	_, err := h.RunBranch("child")
	h.AssertNoError(err)

	// child branch inherits main's commit, so log shows it
	output, err := h.RunLog("child")
	h.AssertNoError(err)
	h.AssertContains(output, "v1 on main")
}
