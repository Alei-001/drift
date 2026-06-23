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

// TC-DIFF-002: Text differences (summary mode)
func TestDiff_TextDifferences_Summary(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "line 1\nline 2\nline 3")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Modify file
	h.WriteFile("note.txt", "line 1\nline two\nline 3")

	output, err := h.RunDiff()
	h.AssertNoError(err)
	// Summary mode shows file status and line counts
	h.AssertContains(output, "M note.txt")
	h.AssertContains(output, "+1 -1")
	h.AssertContains(output, "(text)")
	h.AssertContains(output, "Summary:")
}

// TC-DIFF-002b: Text differences (patch mode)
func TestDiff_TextDifferences_Patch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "line 1\nline 2\nline 3")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Modify file
	h.WriteFile("note.txt", "line 1\nline two\nline 3")

	output, err := h.RunDiffWithPatch()
	h.AssertNoError(err)
	// Patch mode shows detailed diff
	h.AssertContains(output, "---")
	h.AssertContains(output, "+++")
	h.AssertContains(output, "-line 2")
	h.AssertContains(output, "+line two")
}

// TC-DIFF-003: New file (untracked, now shown in worktree diff per Issue 10 fix)
func TestDiff_NewFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Add new file (not committed yet)
	h.WriteFile("new.txt", "new content")

	output, err := h.RunDiff()
	h.AssertNoError(err)
	// Issue 10: untracked files now appear in worktree diff as added.
	h.AssertContains(output, "A new.txt")
}

// TC-DIFF-004: Deleted file (summary mode)
func TestDiff_DeletedFile_Summary(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Delete file
	h.DeleteFile("note.txt")

	output, err := h.RunDiff()
	h.AssertNoError(err)
	// Summary mode shows deleted status
	h.AssertContains(output, "D note.txt")
	h.AssertContains(output, "Summary:")
}

// TC-DIFF-004b: Deleted file (patch mode)
func TestDiff_DeletedFile_Patch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Delete file
	h.DeleteFile("note.txt")

	output, err := h.RunDiffWithPatch()
	h.AssertNoError(err)
	// Patch mode shows deleted content
	h.AssertContains(output, "---")
	h.AssertContains(output, "-content")
}

// TC-DIFF-005: Diff against specific version (summary mode)
func TestDiff_AgainstSpecificVersion_Summary(t *testing.T) {
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

	// Diff against v1 (summary)
	output, err := h.RunDiff("v1")
	h.AssertNoError(err)
	h.AssertContains(output, "M note.txt")
	h.AssertContains(output, "Summary:")
}

// TC-DIFF-005b: Diff against specific version (patch mode)
func TestDiff_AgainstSpecificVersion_Patch(t *testing.T) {
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

	// Diff against v1 (patch)
	output, err := h.RunDiffWithPatch("v1")
	h.AssertNoError(err)
	h.AssertContains(output, "-v1 content")
	h.AssertContains(output, "+worktree content")
}

// TC-DIFF-006: Diff between two versions (summary mode)
func TestDiff_BetweenVersions_Summary(t *testing.T) {
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
	h.AssertContains(output, "M note.txt")
	h.AssertContains(output, "Summary:")
}

// TC-DIFF-006b: Diff between two versions (patch mode)
func TestDiff_BetweenVersions_Patch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create v1
	h.WriteFile("note.txt", "v1 content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Create v2
	h.WriteFile("note.txt", "v2 content")
	h.AddAndSave([]string{"note.txt"}, "v2")

	output, err := h.RunDiffWithPatch("v1", "v2")
	h.AssertNoError(err)
	h.AssertContains(output, "-v1 content")
	h.AssertContains(output, "+v2 content")
}

// TC-DIFF-007: Binary file differences (summary mode)
func TestDiff_BinaryFile_Summary(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create binary file (contains null byte)
	h.WriteFile("image.bin", string([]byte{0, 1, 2, 3, 0, 5, 6, 7}))
	h.AddAndSave([]string{"image.bin"}, "v1")

	// Modify binary file
	h.WriteFile("image.bin", string([]byte{0, 1, 2, 3, 0, 5, 6, 7, 8, 9}))

	output, err := h.RunDiff()
	h.AssertNoError(err)
	// Summary mode shows size change
	h.AssertContains(output, "M image.bin")
	h.AssertContains(output, "(binary)")
	h.AssertContains(output, "8 -> 10 bytes")
}

// TC-DIFF-007b: Binary file differences (patch mode)
func TestDiff_BinaryFile_Patch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create binary file (contains null byte)
	h.WriteFile("image.bin", string([]byte{0, 1, 2, 3, 0, 5, 6, 7}))
	h.AddAndSave([]string{"image.bin"}, "v1")

	// Modify binary file
	h.WriteFile("image.bin", string([]byte{0, 1, 2, 3, 0, 5, 6, 7, 8, 9}))

	output, err := h.RunDiffWithPatch()
	h.AssertNoError(err)
	// Patch mode shows binary file message
	h.AssertContains(output, "Binary file")
	h.AssertContains(output, "changed")
}

// TC-DIFF-008: No versions to compare
func TestDiff_NoVersions(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.RunDiff()
	h.AssertError(err)
}

// TC-DIFF-009: Cross-branch comparison (summary mode)
func TestDiff_CrossBranch_Summary(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create main branch with v1
	h.WriteFile("note.txt", "main content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Create feature branch
	h.RunBranch("feature")
	h.RunSwitch("feature")
	h.WriteFile("note.txt", "feature content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Compare branches (summary)
	output, err := h.RunDiff("main", "feature")
	h.AssertNoError(err)
	h.AssertContains(output, "M note.txt")
	h.AssertContains(output, "Summary:")
}

// TC-DIFF-009b: Cross-branch comparison (patch mode)
func TestDiff_CrossBranch_Patch(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create main branch with v1
	h.WriteFile("note.txt", "main content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Create feature branch
	h.RunBranch("feature")
	h.RunSwitch("feature")
	h.WriteFile("note.txt", "feature content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Compare branches (patch)
	output, err := h.RunDiffWithPatch("main", "feature")
	h.AssertNoError(err)
	h.AssertContains(output, "-main content")
	h.AssertContains(output, "+feature content")
}

// TC-DIFF-010: Specific file filter
func TestDiff_SpecificFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create multiple files
	h.WriteFile("note.txt", "note content")
	h.WriteFile("other.txt", "other content")
	h.AddAndSave([]string{"note.txt", "other.txt"}, "v1")

	// Modify both files
	h.WriteFile("note.txt", "new note")
	h.WriteFile("other.txt", "new other")

	// Diff only note.txt
	output, err := h.RunDiffWithFile("note.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "M note.txt")
	h.AssertNotContains(output, "other.txt")
}

// TC-DIFF-007: Diff with directory filter (worktree vs version)
func TestDiff_DirectoryFilter_Worktree(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("src/main.go", "v1 main")
	h.WriteFile("src/lib/helper.go", "v1 helper")
	h.WriteFile("README.md", "v1 readme")
	h.AddAndSave([]string{"src/main.go", "src/lib/helper.go", "README.md"}, "v1")

	// Modify all files
	h.WriteFile("src/main.go", "v2 main")
	h.WriteFile("src/lib/helper.go", "v2 helper")
	h.WriteFile("README.md", "v2 readme")

	// Diff only src/ directory
	output, err := h.RunDiffWithFile("src/")
	h.AssertNoError(err)
	h.AssertContains(output, "M src/main.go")
	h.AssertContains(output, "M src/lib/helper.go")
	h.AssertNotContains(output, "README.md")
}

// TC-DIFF-008: Diff between versions with file filter
func TestDiff_BetweenVersions_FileFilter(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "v1 a")
	h.WriteFile("b.txt", "v1 b")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v1")

	h.WriteFile("a.txt", "v2 a")
	h.WriteFile("b.txt", "v2 b")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v2")

	// Diff only a.txt between v1 and v2
	output, err := h.RunDiffWithFile("a.txt", "v1", "v2")
	h.AssertNoError(err)
	h.AssertContains(output, "M a.txt")
	h.AssertNotContains(output, "b.txt")
}

// TC-DIFF-009: Diff between versions with directory filter
func TestDiff_BetweenVersions_DirectoryFilter(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("src/main.go", "v1 main")
	h.WriteFile("src/lib/helper.go", "v1 helper")
	h.WriteFile("docs/guide.md", "v1 guide")
	h.AddAndSave([]string{"src/main.go", "src/lib/helper.go", "docs/guide.md"}, "v1")

	h.WriteFile("src/main.go", "v2 main")
	h.WriteFile("src/lib/helper.go", "v2 helper")
	h.WriteFile("docs/guide.md", "v2 guide")
	h.AddAndSave([]string{"src/main.go", "src/lib/helper.go", "docs/guide.md"}, "v2")

	// Diff only src/ between v1 and v2
	output, err := h.RunDiffWithFile("src/", "v1", "v2")
	h.AssertNoError(err)
	h.AssertContains(output, "M src/main.go")
	h.AssertContains(output, "M src/lib/helper.go")
	h.AssertNotContains(output, "docs/guide.md")
}

// TC-DIFF-010: Diff with normalized path (./prefix)
func TestDiff_NormalizedPath(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "v1 a")
	h.WriteFile("b.txt", "v1 b")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v1")

	h.WriteFile("a.txt", "v2 a")
	h.WriteFile("b.txt", "v2 b")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v2")

	// Diff with ./ prefix should normalize and match
	output, err := h.RunDiffWithFile("./a.txt", "v1", "v2")
	h.AssertNoError(err)
	h.AssertContains(output, "M a.txt")
	h.AssertNotContains(output, "b.txt")
}

// TC-DIFF-011: Diff with -- separator for file paths
func TestDiff_DashDashSeparator(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "v1 a")
	h.WriteFile("b.txt", "v1 b")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v1")

	h.WriteFile("a.txt", "v2 a")
	h.WriteFile("b.txt", "v2 b")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v2")

	// drift diff v1 v2 -- a.txt
	// Cobra passes post-"--" args as part of args slice.
	output, err := h.RunDiff("v1", "v2", "a.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "M a.txt")
	h.AssertNotContains(output, "b.txt")
}

// TC-DIFF-012: Diff with -- separator and multiple files
func TestDiff_DashDashMultipleFiles(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "v1 a")
	h.WriteFile("b.txt", "v1 b")
	h.WriteFile("c.txt", "v1 c")
	h.AddAndSave([]string{"a.txt", "b.txt", "c.txt"}, "v1")

	h.WriteFile("a.txt", "v2 a")
	h.WriteFile("b.txt", "v2 b")
	h.WriteFile("c.txt", "v2 c")
	h.AddAndSave([]string{"a.txt", "b.txt", "c.txt"}, "v2")

	// drift diff v1 v2 -- a.txt c.txt
	output, err := h.RunDiff("v1", "v2", "a.txt", "c.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "M a.txt")
	h.AssertContains(output, "M c.txt")
	h.AssertNotContains(output, "b.txt")
}

// TC-DIFF-013: Diff with -f shorthand flag
func TestDiff_ShortFlag(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "v1 a")
	h.WriteFile("b.txt", "v1 b")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v1")

	h.WriteFile("a.txt", "v2 a")
	h.WriteFile("b.txt", "v2 b")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v2")

	// Use -f shorthand by setting the flag directly
	h.SetupSharedState()
	diffFilePaths = []string{"a.txt"}
	output, err := CaptureOutput(func() error {
		return diffCmd.RunE(diffCmd, []string{"v1", "v2"})
	})
	h.AssertNoError(err)
	h.AssertContains(output, "M a.txt")
	h.AssertNotContains(output, "b.txt")
}

// TC-DIFF-014: Diff with -- separator and directory filter
func TestDiff_DashDashDirectory(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("src/main.go", "v1 main")
	h.WriteFile("src/lib/helper.go", "v1 helper")
	h.WriteFile("docs/guide.md", "v1 guide")
	h.AddAndSave([]string{"src/main.go", "src/lib/helper.go", "docs/guide.md"}, "v1")

	h.WriteFile("src/main.go", "v2 main")
	h.WriteFile("src/lib/helper.go", "v2 helper")
	h.WriteFile("docs/guide.md", "v2 guide")
	h.AddAndSave([]string{"src/main.go", "src/lib/helper.go", "docs/guide.md"}, "v2")

	// drift diff v1 v2 -- src/
	output, err := h.RunDiff("v1", "v2", "src/")
	h.AssertNoError(err)
	h.AssertContains(output, "M src/main.go")
	h.AssertContains(output, "M src/lib/helper.go")
	h.AssertNotContains(output, "docs/guide.md")
}
