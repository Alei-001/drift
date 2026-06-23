package cli

import (
	"testing"

	"github.com/drift/drift/internal/core"
)

// TC-ADD-001: Add single file
func TestAdd_SingleFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "hello world")
	output, err := h.RunAdd("note.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Added: note.txt")

	// Verify index contains the file
	var idx core.Index
	h.Store.LoadIndex(&idx)
	if !idx.Has("note.txt") {
		t.Error("index should contain note.txt")
	}
}

// TC-ADD-002: Add multiple files
func TestAdd_MultipleFiles(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "file a")
	h.WriteFile("b.txt", "file b")

	output, err := h.RunAdd("a.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Added: a.txt")

	output, err = h.RunAdd("b.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Added: b.txt")
}

// TC-ADD-003: Add directory
func TestAdd_Directory(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("chapter/ch1.txt", "chapter 1")
	h.WriteFile("chapter/ch2.txt", "chapter 2")

	output, err := h.RunAdd("chapter/")
	h.AssertNoError(err)
	h.AssertContains(output, "Added 2 file(s)")
}

// TC-ADD-004: Add current directory
func TestAdd_All(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "file a")
	h.WriteFile("b.txt", "file b")
	h.WriteFile("c.txt", "file c")

	output, err := h.RunAdd(".")
	h.AssertNoError(err)
	h.AssertContains(output, "Added 3 file(s)")
}

// TC-ADD-005: Add nonexistent path
func TestAdd_NonexistentPath(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.RunAdd("nonexistent.txt")
	h.AssertError(err)
}

// TC-ADD-006: Re-add modified file
func TestAdd_ReAddModified(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "original")
	_, err := h.RunAdd("note.txt")
	h.AssertNoError(err)

	h.WriteFile("note.txt", "modified")
	output, err := h.RunAdd("note.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Added: note.txt")
}

// TC-STATUS-001: Empty working tree (no files, no staging)
func TestStatus_EmptyClean(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	output, err := h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "Nothing to commit, working tree clean")
}

// TC-STATUS-002: Staged new file
func TestStatus_StagedNewFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	_, err := h.RunAdd("note.txt")
	h.AssertNoError(err)

	output, err := h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "Staged changes:")
	h.AssertContains(output, "A note.txt")
}

// TC-STATUS-003: Staged modified file
func TestStatus_StagedModifiedFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create and save v1
	h.WriteFile("note.txt", "original")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Modify and stage
	h.WriteFile("note.txt", "modified")
	_, err := h.RunAdd("note.txt")
	h.AssertNoError(err)

	output, err := h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "Staged changes:")
	h.AssertContains(output, "M note.txt")
}

// TC-STATUS-004: Staged deleted file
func TestStatus_StagedDeletedFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create and save v1
	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Delete file
	h.DeleteFile("note.txt")

	// Status should show deleted file (unstaged)
	output, err := h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "Unstaged changes:")
	h.AssertContains(output, "D note.txt")
}

// TC-STATUS-005: Unstaged modification
func TestStatus_UnstagedModification(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create and save v1
	h.WriteFile("note.txt", "original")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Modify without staging
	h.WriteFile("note.txt", "modified")

	output, err := h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "Unstaged changes:")
	h.AssertContains(output, "M note.txt")
}

// TC-STATUS-006: Untracked file
func TestStatus_UntrackedFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("new.txt", "new file")

	output, err := h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "Untracked files:")
	h.AssertContains(output, "new.txt")
}

// TC-STATUS-007: Mixed status
func TestStatus_MixedStatus(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create and save v1 with old.txt
	h.WriteFile("old.txt", "old content")
	h.AddAndSave([]string{"old.txt"}, "v1")

	// Stage a new file
	h.WriteFile("staged.txt", "staged content")
	_, err := h.RunAdd("staged.txt")
	h.AssertNoError(err)

	// Modify old.txt without staging
	h.WriteFile("old.txt", "modified old")

	// Create untracked file
	h.WriteFile("untracked.txt", "untracked")

	output, err := h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "Staged changes:")
	h.AssertContains(output, "A staged.txt")
	h.AssertContains(output, "Unstaged changes:")
	h.AssertContains(output, "M old.txt")
	h.AssertContains(output, "Untracked files:")
	h.AssertContains(output, "untracked.txt")
}

// TC-UNSTAGE-001: Clear staging area
func TestUnstage_ClearStaging(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	_, err := h.RunAdd("note.txt")
	h.AssertNoError(err)

	output, err := h.RunUnstage()
	h.AssertNoError(err)
	h.AssertContains(output, "Staging area cleared")

	// Verify index is empty - file is now untracked
	output, err = h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "Untracked files:")
	h.AssertContains(output, "note.txt")
}

// TC-ADD-009: CRLF→LF conversion when autocrlf is set
func TestAdd_AutoCRLF(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.Config.Core.AutoCRLF = "true"
	h.SetupSharedState()

	crlfContent := "line1\r\nline2\r\nline3\r\n"
	h.WriteFile("note.txt", crlfContent)

	_, err := h.RunAdd("note.txt")
	h.AssertNoError(err)

	var idx core.Index
	h.Store.LoadIndex(&idx)
	entry, err := idx.Entry("note.txt")
	if err != nil {
		t.Fatalf("index entry not found: %v", err)
	}

	data, err := h.Store.GetBlob(entry.Hash)
	h.AssertNoError(err)

	lfContent := "line1\nline2\nline3\n"
	if string(data) != lfContent {
		t.Errorf("stored content = %q, want %q (LF normalized)", string(data), lfContent)
	}
}

// TC-ADD-010: No conversion when autocrlf is empty
func TestAdd_AutoCRLF_Default(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	crlfContent := "line1\r\nline2\r\nline3\r\n"
	h.WriteFile("note.txt", crlfContent)

	_, err := h.RunAdd("note.txt")
	h.AssertNoError(err)

	var idx core.Index
	h.Store.LoadIndex(&idx)
	entry, err := idx.Entry("note.txt")
	if err != nil {
		t.Fatalf("index entry not found: %v", err)
	}

	data, err := h.Store.GetBlob(entry.Hash)
	h.AssertNoError(err)

	if string(data) != crlfContent {
		t.Errorf("stored content = %q, want %q (CRLF preserved)", string(data), crlfContent)
	}
}

// TC-ADD-011: Binary files skip conversion even when autocrlf is set
func TestAdd_AutoCRLF_BinarySkipped(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.Config.Core.AutoCRLF = "true"
	h.SetupSharedState()

	binaryContent := string([]byte{0, 1, 2, 'a', '\r', '\n', 'b'})
	h.WriteFile("data.bin", binaryContent)

	_, err := h.RunAdd("data.bin")
	h.AssertNoError(err)

	var idx core.Index
	h.Store.LoadIndex(&idx)
	entry, err := idx.Entry("data.bin")
	if err != nil {
		t.Fatalf("index entry not found: %v", err)
	}

	data, err := h.Store.GetBlob(entry.Hash)
	h.AssertNoError(err)

	if string(data) != binaryContent {
		t.Errorf("binary content should be preserved, got %x, want %x", data, []byte(binaryContent))
	}
}

// TC-ADD-007: Re-adding unchanged file skips it
func TestAdd_SkipUnchangedFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	output, err := h.RunAdd("note.txt")
	h.AssertNoError(err)
	if output != "" {
		t.Errorf("expected empty output for unchanged file, got %q", output)
	}
}

// TC-ADD-008: Re-adding file with same hash already in index skips it
func TestAdd_SkipDuplicateInIndex(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	_, err := h.RunAdd("note.txt")
	h.AssertNoError(err)

	output, err := h.RunAdd("note.txt")
	h.AssertNoError(err)
	if output != "" {
		t.Errorf("expected empty output for already-staged identical file, got %q", output)
	}
}

// TC-UNSTAGE-002: Unstage on empty staging area
func TestUnstage_EmptyStaging(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	output, err := h.RunUnstage()
	h.AssertNoError(err)
	h.AssertContains(output, "Staging area cleared")
}

// TC-UNSTAGE-003: Unstage single file from staging
func TestUnstage_SingleFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "file a")
	h.WriteFile("b.txt", "file b")
	_, err := h.RunAdd("a.txt")
	h.AssertNoError(err)
	_, err = h.RunAdd("b.txt")
	h.AssertNoError(err)

	// Verify both are staged
	output, err := h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "A a.txt")
	h.AssertContains(output, "A b.txt")

	// Unstage a.txt only
	output, err = h.RunUnstage("a.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Unstaged: a.txt")

	// Verify a.txt is no longer staged
	output, err = h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "Untracked files:")
	h.AssertContains(output, "a.txt")
	h.AssertNotContains(output, "A a.txt")
	// b.txt should still be staged
	h.AssertContains(output, "A b.txt")
}

// TC-UNSTAGE-004: Unstage single file not in staging
func TestUnstage_FileNotStaged(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")

	output, err := h.RunUnstage("note.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "note.txt is not staged")
}

// TC-UNSTAGE-005: Unstage with invalid path
func TestUnstage_InvalidPath(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Path with traversal (rejected by ValidateTreePath)
	_, err := h.RunUnstage("../outside.txt")
	h.AssertError(err)

	// Path with null byte (rejected by ValidateTreePath)
	_, err = h.RunUnstage("file\x00.txt")
	h.AssertError(err)
}
