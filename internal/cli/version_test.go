package cli

import (
	"regexp"
	"strings"
	"testing"
)

// TC-SAVE-001: Save without message
func TestSave_WithoutMessage(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	_, err := h.RunAdd("note.txt")
	h.AssertNoError(err)

	output, err := h.RunSave("")
	h.AssertNoError(err)
	h.AssertContains(output, "Saved version v1")

	// Verify commit exists
	if h.VersionCount() != 1 {
		t.Errorf("expected 1 commit, got %d", h.VersionCount())
	}
}

// TC-SAVE-002: Save with message
func TestSave_WithMessage(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("ch1.txt", "chapter 1")
	_, err := h.RunAdd("ch1.txt")
	h.AssertNoError(err)

	output, err := h.RunSave("first chapter")
	h.AssertNoError(err)
	h.AssertContains(output, "Saved version v1: first chapter")
}

// TC-LOG-004: Filter by branch name (formerly TestList_FilterByBranch)
func TestLog_FilterByBranch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create v1 on main
	h.WriteFile("main.txt", "main content")
	h.AddAndSave([]string{"main.txt"}, "v1")

	// Create feature branch with v1
	_, err := h.RunBranch("feature")
	h.AssertNoError(err)
	_, err = h.RunSwitch("feature")
	h.AssertNoError(err)
	h.WriteFile("feat.txt", "feature content")
	h.AddAndSave([]string{"feat.txt"}, "v1")

	// Log only main branch
	output, err := h.RunLog("main")
	h.AssertNoError(err)
	h.AssertContains(output, "Version: v1")
	h.AssertContains(output, "Branch:  main")
	h.AssertNotContains(output, "feature")
}

// TC-LOG-005: Log nonexistent branch errors (formerly TestList_NonexistentBranch)
func TestLog_NonexistentBranch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	_, err := h.RunLog("nonexistent")
	h.AssertError(err)
}

// TC-LOG-006: Deduplicate commits across branches with --all (formerly TestList_DeduplicateAcrossBranches)
func TestLog_AllDeduplicateAcrossBranches(t *testing.T) {
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

	// Log --all should show v1 only once
	output, err := h.RunLogAll()
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

// TC-SAVE-006: Prevent empty commit (same tree hash as parent)
func TestSave_PreventsEmptyCommit(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Stage same file with same content
	h.WriteFile("note.txt", "content")
	_, err := h.RunAdd("note.txt")
	h.AssertNoError(err)

	_, err = h.RunSave("should fail")
	h.AssertError(err)
}

// TC-SAVE-007: Shows saved files list
func TestSave_ShowsSavedFiles(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "file a")
	h.WriteFile("b.txt", "file b")
	_, err := h.RunAdd(".")
	h.AssertNoError(err)

	output, err := h.RunSave("multi file")
	h.AssertNoError(err)
	h.AssertContains(output, "2 file(s) saved:")
	h.AssertContains(output, "a.txt")
	h.AssertContains(output, "b.txt")
}

// TC-SAVE-008: Save empty staging area
func TestSave_EmptyStaging(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.RunSave("")
	h.AssertError(err)
}

// TC-SAVE-009: Save with special characters in message
func TestSave_SpecialCharsInMessage(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Unicode characters
	h.WriteFile("note.txt", "content v2")
	h.AddAndSave([]string{"note.txt"}, "章节 1：开始 🎨")

	// Save with multi-line message
	h.WriteFile("note.txt", "content v3")
	_, err := h.RunAdd("note.txt")
	h.AssertNoError(err)
	output, err := h.RunSave("line1\nline2\nline3")
	h.AssertNoError(err)
	h.AssertContains(output, "Saved version v3")
}

// TC-SAVE-004: Version number auto-increment
func TestSave_VersionIncrement(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Save v1
	h.WriteFile("f1.txt", "v1")
	h.AddAndSave([]string{"f1.txt"}, "v1")

	// Save v2
	h.WriteFile("f2.txt", "v2")
	h.AddAndSave([]string{"f2.txt"}, "v2")

	// Save v3
	h.WriteFile("f3.txt", "v3")
	h.AddAndSave([]string{"f3.txt"}, "v3")

	if h.VersionCount() != 3 {
		t.Errorf("expected 3 commits, got %d", h.VersionCount())
	}
}

// TC-SAVE-005: Status clean after save
func TestSave_StatusCleanAfterSave(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	output, err := h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "Nothing to commit, working tree clean")
}

// TC-LOG-007: Show version history with --all (formerly TestList_ShowHistory)
func TestLog_AllShowHistory(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create 3 versions
	h.WriteFile("f1.txt", "v1")
	h.AddAndSave([]string{"f1.txt"}, "v1")

	h.WriteFile("f2.txt", "v2")
	h.AddAndSave([]string{"f2.txt"}, "v2")

	h.WriteFile("f3.txt", "v3")
	h.AddAndSave([]string{"f3.txt"}, "v3")

	output, err := h.RunLogAll()
	h.AssertNoError(err)
	h.AssertContains(output, "Version history:")
	h.AssertContains(output, "v3")
	h.AssertContains(output, "v2")
	h.AssertContains(output, "v1")
}

// TC-LOG-008: No versions yet with --all (formerly TestList_NoVersions)
func TestLog_AllNoVersions(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	output, err := h.RunLogAll()
	h.AssertNoError(err)
	h.AssertContains(output, "No versions yet")
}

// TC-LOG-009: Version without message with --all (formerly TestList_NoMessage)
func TestLog_AllNoMessage(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "")

	output, err := h.RunLogAll()
	h.AssertNoError(err)
	h.AssertContains(output, "v1")
	// When message is empty, the line should end with the branch (no trailing message).
	// Format: "  v1  [main]  <message>"
	// Use regex to extract the message part after the branch.
	re := regexp.MustCompile(`v\d+\s+\[.*?\]\s*(.*)`)
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
