package cli

import (
	"testing"
)

// TC-DIFF-001: No differences (worktree vs latest)
func TestDiff_NoDifferences(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	output, err := h.RunDiff()
	h.AssertNoError(err)
	h.AssertContains(output, "No differences")
}

// TC-DIFF-002: Text differences
func TestDiff_TextDifferences(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "line 1\nline 2\nline 3")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Modify file
	h.WriteFile("note.txt", "line 1\nline two\nline 3")

	output, err := h.RunDiff()
	h.AssertNoError(err)
	h.AssertContains(output, "--- note.txt")
	h.AssertContains(output, "+++ note.txt")
	h.AssertContains(output, "-line 2")
	h.AssertContains(output, "+line two")
}

// TC-DIFF-003: New file (not tracked in version, won't show in diff)
func TestDiff_NewFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Add new file (not committed yet)
	h.WriteFile("new.txt", "new content")

	output, err := h.RunDiff()
	h.AssertNoError(err)
	// New uncommitted files don't appear in diff (only tracked files)
	h.AssertContains(output, "No differences")
}

// TC-DIFF-004: Deleted file
func TestDiff_DeletedFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Delete file
	h.DeleteFile("note.txt")

	output, err := h.RunDiff()
	h.AssertNoError(err)
	h.AssertContains(output, "--- note.txt")
	h.AssertContains(output, "+++ /dev/null")
	h.AssertContains(output, "(deleted)")
}

// TC-DIFF-005: Diff against specific version
func TestDiff_AgainstSpecificVersion(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create v1
	h.WriteFile("note.txt", "v1 content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Create v2
	h.WriteFile("note.txt", "v2 content")
	h.AddAndSave([]string{"note.txt"}, "v2")

	// Modify worktree
	h.WriteFile("note.txt", "worktree content")

	// Diff against v1
	output, err := h.RunDiff("v1")
	h.AssertNoError(err)
	h.AssertContains(output, "-v1 content")
	h.AssertContains(output, "+worktree content")
}

// TC-DIFF-006: Diff between two versions
func TestDiff_BetweenVersions(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create v1
	h.WriteFile("note.txt", "v1 content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Create v2
	h.WriteFile("note.txt", "v2 content")
	h.AddAndSave([]string{"note.txt"}, "v2")

	output, err := h.RunDiff("v1", "v2")
	h.AssertNoError(err)
	h.AssertContains(output, "-v1 content")
	h.AssertContains(output, "+v2 content")
}

// TC-DIFF-007: Binary file differences
func TestDiff_BinaryFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create binary file (contains null byte)
	h.WriteFile("image.bin", string([]byte{0, 1, 2, 3, 0, 5, 6, 7}))
	h.AddAndSave([]string{"image.bin"}, "v1")

	// Modify binary file
	h.WriteFile("image.bin", string([]byte{0, 1, 2, 3, 0, 5, 6, 7, 8, 9}))

	output, err := h.RunDiff()
	h.AssertNoError(err)
	h.AssertContains(output, "Binary file changed")
}

// TC-DIFF-008: No versions to compare
func TestDiff_NoVersions(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.RunDiff()
	h.AssertError(err)
}
