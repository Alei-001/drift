package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TC-EXPORT-001: Export to directory
func TestExport_ToDirectory(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "hello world")
	h.AddAndSave([]string{"note.txt"}, "v1")

	outputDir := filepath.Join(h.Dir, "output")
	output, err := h.RunExport("v1", "-o", outputDir)
	h.AssertNoError(err)
	h.AssertContains(output, "Exported 1 file(s)")

	// Verify exported file
	exportedFile := filepath.Join(outputDir, "note.txt")
	data, err := os.ReadFile(exportedFile)
	h.AssertNoError(err)
	if string(data) != "hello world" {
		t.Errorf("exported file content = %q, want %q", string(data), "hello world")
	}
}

// TC-EXPORT-002: Export to zip
func TestExport_ToZip(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	outputFile := filepath.Join(h.Dir, "output.zip")
	output, err := h.RunExport("v1", "-o", outputFile, "-f", "zip")
	h.AssertNoError(err)
	h.AssertContains(output, "Exported 1 file(s)")

	// Verify zip file exists
	if !h.FileExists("output.zip") {
		t.Error("zip file should exist")
	}
}

// TC-EXPORT-003: Export to tar.gz
func TestExport_ToTarGz(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	outputFile := filepath.Join(h.Dir, "output.tar.gz")
	output, err := h.RunExport("v1", "-o", outputFile, "-f", "tar")
	h.AssertNoError(err)
	h.AssertContains(output, "Exported 1 file(s)")

	// Verify tar.gz file exists
	if !h.FileExists("output.tar.gz") {
		t.Error("tar.gz file should exist")
	}
}

// TC-EXPORT-004: Missing -o flag
func TestExport_MissingOutput(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	_, err := h.RunExport("v1")
	h.AssertError(err)
}

// TC-EXPORT-005: Version not found
func TestExport_VersionNotFound(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	outputDir := filepath.Join(h.Dir, "output")
	_, err := h.RunExport("v99", "-o", outputDir)
	h.AssertError(err)
}

// TC-EXPORT-006: Output directory already exists
func TestExport_DirAlreadyExists(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Create output directory
	outputDir := filepath.Join(h.Dir, "output")
	os.MkdirAll(outputDir, 0755)

	_, err := h.RunExport("v1", "-o", outputDir)
	h.AssertError(err)
}

// TC-RESTORE-001: Restore to specific version
func TestRestore_ToVersion(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create v1
	h.WriteFile("a.txt", "v1 content")
	h.AddAndSave([]string{"a.txt"}, "v1")

	// Create v2 with modified file and new file
	h.WriteFile("a.txt", "v2 content")
	h.WriteFile("b.txt", "new in v2")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v2")

	// Restore to v1
	output, err := h.RunRestore("v1")
	h.AssertNoError(err)
	h.AssertContains(output, "Restored to v1")

	// Verify a.txt has v1 content
	content := h.ReadFile("a.txt")
	if content != "v1 content" {
		t.Errorf("a.txt content = %q, want %q", content, "v1 content")
	}

	// Verify b.txt is deleted (v1 didn't have it)
	if h.FileExists("b.txt") {
		t.Error("b.txt should be deleted after restore to v1")
	}
}

// TC-RESTORE-002: Restore with staged changes (no --force)
func TestRestore_StagedChangesNoForce(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create v1
	h.WriteFile("a.txt", "v1")
	h.AddAndSave([]string{"a.txt"}, "v1")

	// Stage a change
	h.WriteFile("a.txt", "staged")
	_, err := h.RunAdd("a.txt")
	h.AssertNoError(err)

	// Restore should fail without --force
	_, err = h.RunRestore("v1")
	h.AssertError(err)
}

// TC-RESTORE-003: Restore with staged changes and --force
func TestRestore_StagedChangesForce(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create v1
	h.WriteFile("a.txt", "v1")
	h.AddAndSave([]string{"a.txt"}, "v1")

	// Create v2
	h.WriteFile("a.txt", "v2")
	h.AddAndSave([]string{"a.txt"}, "v2")

	// Stage a change
	h.WriteFile("a.txt", "staged")
	_, err := h.RunAdd("a.txt")
	h.AssertNoError(err)

	// Restore should succeed with --force
	output, err := h.RunRestore("v1", "--force")
	h.AssertNoError(err)
	h.AssertContains(output, "Restored to v1")
}

// TC-RESTORE-004: Preserve untracked files
func TestRestore_PreserveUntracked(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create v1
	h.WriteFile("a.txt", "v1")
	h.AddAndSave([]string{"a.txt"}, "v1")

	// Create untracked file
	h.WriteFile("untracked.txt", "keep me")

	// Restore v1 (should not delete untracked)
	output, err := h.RunRestore("v1")
	h.AssertNoError(err)
	h.AssertContains(output, "Restored to v1")

	// Verify untracked file preserved
	if !h.FileExists("untracked.txt") {
		t.Error("untracked.txt should be preserved")
	}
	content := h.ReadFile("untracked.txt")
	if content != "keep me" {
		t.Errorf("untracked.txt content = %q, want %q", content, "keep me")
	}
}
