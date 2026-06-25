package cli

import (
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
	id1 := h.ExtractSaveID(output)
	h.AssertContains(output, "Saved version "+id1)

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
	id1 := h.ExtractSaveID(output)
	h.AssertContains(output, "Saved version "+id1+": first chapter")
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
	id3 := h.ExtractSaveID(output)
	h.AssertContains(output, "Saved version "+id3)
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

// TC-SAVE-010: save --all auto-stages changes before saving
func TestSave_AllAutoStage(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// v1 with one file (explicit add)
	h.WriteFile("a.txt", "content a")
	h.AddAndSave([]string{"a.txt"}, "v1")

	// Modify existing + add new file, but do NOT run `drift add`
	h.WriteFile("a.txt", "modified a")
	h.WriteFile("b.txt", "content b")

	// save --all should auto-stage both files and create v2
	output, err := h.RunSaveAll("auto stage")
	h.AssertNoError(err)
	id2 := h.ExtractSaveID(output)
	h.AssertContains(output, "Saved version "+id2+": auto stage")
	// Both files should appear in the saved list.
	h.AssertContains(output, "a.txt")
	h.AssertContains(output, "b.txt")

	if h.VersionCount() != 2 {
		t.Errorf("expected 2 commits, got %d", h.VersionCount())
	}

	// Working tree should be clean after save --all.
	statusOutput, err := h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(statusOutput, "Nothing to commit, working tree clean")
}

// TC-SAVE-011: save --all with no changes still errors (nothing to save)
func TestSave_AllNoChanges(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "content a")
	h.AddAndSave([]string{"a.txt"}, "v1")

	// No modifications — save --all should fail with nothing to save.
	_, err := h.RunSaveAll("nothing")
	h.AssertError(err)
}
