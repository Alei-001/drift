package cli

import (
	"testing"
)

// TC-ERR-001: Uninitialized add
func TestErr_UninitializedAdd(t *testing.T) {
	h := NewTestHelper(t)

	_, err := h.RunAdd("note.txt")
	h.AssertError(err)
}

// TC-ERR-002: Uninitialized save
func TestErr_UninitializedSave(t *testing.T) {
	h := NewTestHelper(t)

	_, err := h.RunSave("")
	h.AssertError(err)
}

// TC-ERR-003: Uninitialized history --all
func TestErr_UninitializedHistoryAll(t *testing.T) {
	h := NewTestHelper(t)

	_, err := h.RunHistoryAll()
	h.AssertError(err)
}

// TC-ERR-004: Version not found for restore/export/diff
func TestErr_VersionNotFound(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Restore nonexistent version
	_, err := h.RunRestore("v99")
	h.AssertError(err)

	// Export nonexistent version
	_, err = h.RunExport("v99", "-o", "output")
	h.AssertError(err)

	// Diff nonexistent version
	_, err = h.RunDiff("v99")
	h.AssertError(err)
}

// TC-EDGE-001: Empty file
func TestEdge_EmptyFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create empty file
	h.WriteFile("empty.txt", "")

	// Add and save
	output, err := h.RunAdd("empty.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Added: empty.txt")

	output, err = h.RunSave("empty file")
	h.AssertNoError(err)
	id1 := h.ExtractSaveID(output)
	h.AssertContains(output, "Saved version "+id1)

	// Verify blob exists
	if h.VersionCount() != 1 {
		t.Errorf("expected 1 commit, got %d", h.VersionCount())
	}
}

// TC-EDGE-002: Filename with spaces
func TestEdge_FilenameWithSpaces(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("my file.txt", "content")

	output, err := h.RunAdd("my file.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Added: my file.txt")

	output, err = h.RunSave("spaces")
	h.AssertNoError(err)
	id1 := h.ExtractSaveID(output)
	h.AssertContains(output, "Saved version "+id1)
}

// TC-EDGE-003: Chinese filename
func TestEdge_ChineseFilename(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("笔记.txt", "content")

	output, err := h.RunAdd("笔记.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Added: 笔记.txt")

	output, err = h.RunSave("chinese")
	h.AssertNoError(err)
	id1 := h.ExtractSaveID(output)
	h.AssertContains(output, "Saved version "+id1)
}

// TC-EDGE-004: Deep nested directory
func TestEdge_DeepNestedDirectory(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a/b/c/d/file.txt", "deep")

	output, err := h.RunAdd("a/")
	h.AssertNoError(err)
	h.AssertContains(output, "Added 1 file(s)")

	output, err = h.RunSave("deep nesting")
	h.AssertNoError(err)
	id1 := h.ExtractSaveID(output)
	h.AssertContains(output, "Saved version "+id1)
}

// TC-EDGE-005: Many files
func TestEdge_ManyFiles(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create 100 files
	for i := 0; i < 100; i++ {
		h.WriteFile("file_"+string(rune('a'+i%26))+".txt", "content")
	}

	output, err := h.RunAdd(".")
	h.AssertNoError(err)
	h.AssertContains(output, "Added")

	output, err = h.RunSave("100 files")
	h.AssertNoError(err)
	id1 := h.ExtractSaveID(output)
	h.AssertContains(output, "Saved version "+id1)
}

// TC-EDGE-006: Same content different filenames
func TestEdge_SameContentDifferentNames(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "same content")
	h.WriteFile("b.txt", "same content")

	_, err := h.RunAdd("a.txt")
	h.AssertNoError(err)
	_, err = h.RunAdd("b.txt")
	h.AssertNoError(err)

	output, err := h.RunSave("same content")
	h.AssertNoError(err)
	id1 := h.ExtractSaveID(output)
	h.AssertContains(output, "Saved version "+id1)

	// Both files should be tracked
	output, err = h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "Nothing to commit, working tree clean")
}

// TC-EDGE-007: File modification changes hash
func TestEdge_FileModificationChangesHash(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create and save v1
	h.WriteFile("note.txt", "original")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Get v1 commit's tree hash
	commits1, err := h.Store.ListCommits()
	h.AssertNoError(err)
	treeHash1 := commits1[0].TreeHash

	// Modify and save v2
	h.WriteFile("note.txt", "modified")
	h.AddAndSave([]string{"note.txt"}, "v2")

	// Get v2 commit's tree hash
	commits2, err := h.Store.ListCommits()
	h.AssertNoError(err)
	treeHash2 := commits2[1].TreeHash

	// Tree hashes should be different
	if treeHash1 == treeHash2 {
		t.Errorf("treeHash1 = %q should not equal treeHash2 = %q", treeHash1, treeHash2)
	}
}
