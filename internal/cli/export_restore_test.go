package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// TC-EXPORT-007: Export zip file already exists
func TestExport_ZipAlreadyExists(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Create the zip file first
	zipPath := filepath.Join(h.Dir, "output.zip")
	os.WriteFile(zipPath, []byte("dummy"), 0644)

	_, err := h.RunExport("v1", "-o", zipPath, "-f", "zip")
	h.AssertError(err)
}

// TC-EXPORT-008: Export tar.gz file already exists
func TestExport_TarGzAlreadyExists(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Create the tar.gz file first
	tarPath := filepath.Join(h.Dir, "output.tar.gz")
	os.WriteFile(tarPath, []byte("dummy"), 0644)

	_, err := h.RunExport("v1", "-o", tarPath, "-f", "tar")
	h.AssertError(err)
}

// TC-EXPORT-009: Export single file by path
func TestExport_SingleFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("src/main.go", "package main")
	h.WriteFile("src/util.go", "package util")
	h.WriteFile("README.md", "readme")
	h.AddAndSave([]string{"src/main.go", "src/util.go", "README.md"}, "v1")

	outputDir := filepath.Join(h.Dir, "output")
	output, err := h.RunExport("v1", "-o", outputDir, "src/main.go")
	h.AssertNoError(err)
	h.AssertContains(output, "Exported 1 file(s)")

	// Verify only main.go was exported
	if !h.FileExists(filepath.Join("output", "src", "main.go")) {
		t.Error("src/main.go should exist in output")
	}
	if h.FileExists(filepath.Join("output", "src", "util.go")) {
		t.Error("src/util.go should NOT exist in output")
	}
	if h.FileExists(filepath.Join("output", "README.md")) {
		t.Error("README.md should NOT exist in output")
	}
}

// TC-EXPORT-010: Export directory subtree
func TestExport_Directory(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("src/main.go", "package main")
	h.WriteFile("src/lib/helper.go", "package lib")
	h.WriteFile("README.md", "readme")
	h.AddAndSave([]string{"src/main.go", "src/lib/helper.go", "README.md"}, "v1")

	outputDir := filepath.Join(h.Dir, "output")
	output, err := h.RunExport("v1", "-o", outputDir, "src/")
	h.AssertNoError(err)
	h.AssertContains(output, "Exported 2 file(s)")

	// Verify only files under src/ were exported
	if !h.FileExists(filepath.Join("output", "src", "main.go")) {
		t.Error("src/main.go should exist in output")
	}
	if !h.FileExists(filepath.Join("output", "src", "lib", "helper.go")) {
		t.Error("src/lib/helper.go should exist in output")
	}
	if h.FileExists(filepath.Join("output", "README.md")) {
		t.Error("README.md should NOT exist in output")
	}
}

// TC-EXPORT-011: Export multiple paths
func TestExport_MultiplePaths(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("src/main.go", "package main")
	h.WriteFile("src/util.go", "package util")
	h.WriteFile("docs/guide.md", "guide")
	h.WriteFile("README.md", "readme")
	h.AddAndSave([]string{"src/main.go", "src/util.go", "docs/guide.md", "README.md"}, "v1")

	outputDir := filepath.Join(h.Dir, "output")
	output, err := h.RunExport("v1", "-o", outputDir, "src/main.go", "docs/")
	h.AssertNoError(err)
	h.AssertContains(output, "Exported 2 file(s)")

	if !h.FileExists(filepath.Join("output", "src", "main.go")) {
		t.Error("src/main.go should exist in output")
	}
	if !h.FileExists(filepath.Join("output", "docs", "guide.md")) {
		t.Error("docs/guide.md should exist in output")
	}
	if h.FileExists(filepath.Join("output", "README.md")) {
		t.Error("README.md should NOT exist in output")
	}
	if h.FileExists(filepath.Join("output", "src", "util.go")) {
		t.Error("src/util.go should NOT exist in output")
	}
}

// TC-EXPORT-012: Export with nonexistent path errors
func TestExport_PathNotFound(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	outputDir := filepath.Join(h.Dir, "output")
	_, err := h.RunExport("v1", "-o", outputDir, "nonexistent.txt")
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

// TC-RESTORE-005: Restore with autocrlf=true converts LF→CRLF on Windows
func TestRestore_AutoCRLF_LFtoCRLF(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create file with LF line endings (as stored internally)
	h.WriteFile("note.txt", "line1\nline2\nline3\n")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// Enable autocrlf
	h.Config.Core.AutoCRLF = "true"
	h.SetupSharedState()

	// Restore
	_, err := h.RunRestore("v1", "--force")
	h.AssertNoError(err)

	data, err := os.ReadFile(filepath.Join(h.Dir, "note.txt"))
	h.AssertNoError(err)

	if runtime.GOOS == "windows" {
		// On Windows, LF should be converted to CRLF
		if !strings.Contains(string(data), "\r\n") {
			t.Errorf("expected CRLF line endings on Windows, got: %q", string(data))
		}
		if strings.Count(string(data), "\r\n") != 3 {
			t.Errorf("expected 3 CRLF sequences on Windows, got: %q", string(data))
		}
	} else {
		// On other platforms, LF should be preserved
		if strings.Contains(string(data), "\r\n") {
			t.Errorf("expected LF line endings on non-Windows, got: %q", string(data))
		}
	}
}

// TC-RESTORE-006: Restore with autocrlf=false preserves LF
func TestRestore_AutoCRLF_Off_PreservesLF(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "line1\nline2\nline3\n")
	h.AddAndSave([]string{"note.txt"}, "v1")

	// autocrlf is off by default
	h.SetupSharedState()

	_, err := h.RunRestore("v1", "--force")
	h.AssertNoError(err)

	data, err := os.ReadFile(filepath.Join(h.Dir, "note.txt"))
	h.AssertNoError(err)

	// LF should always be preserved when autocrlf is off
	if strings.Contains(string(data), "\r\n") {
		t.Errorf("expected LF line endings when autocrlf is off, got: %q", string(data))
	}
}

// TC-RESTORE-007: Full round trip CRLF→LF→CRLF with autocrlf=true
func TestRestore_AutoCRLF_RoundTrip(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Original content has CRLF endings
	original := "line1\r\nline2\r\nline3\r\n"

	h.Config.Core.AutoCRLF = "true"
	h.SetupSharedState()

	h.WriteFile("note.txt", original)

	// Add (CRLF→LF stored)
	_, err := h.RunAdd("note.txt")
	h.AssertNoError(err)

	// Save
	_, err = h.RunSave("v1")
	h.AssertNoError(err)

	// Restore
	_, err = h.RunRestore("v1", "--force")
	h.AssertNoError(err)

	data, err := os.ReadFile(filepath.Join(h.Dir, "note.txt"))
	h.AssertNoError(err)

	if runtime.GOOS == "windows" {
		// On Windows, should return to CRLF
		if string(data) != original {
			t.Errorf("round-trip failed on Windows:\n  got:  %q\n  want: %q", string(data), original)
		}
	} else {
		// On other platforms, stored LF is preserved
		if strings.Contains(string(data), "\r\n") {
			t.Errorf("expected LF on non-Windows after round-trip, got: %q", string(data))
		}
	}
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

// TC-RESTORE-008: Partial restore of a single file
func TestRestore_SingleFile(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// v1: a.txt and b.txt
	h.WriteFile("a.txt", "v1 a")
	h.WriteFile("b.txt", "v1 b")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v1")

	// v2: modify both files
	h.WriteFile("a.txt", "v2 a")
	h.WriteFile("b.txt", "v2 b")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v2")

	// Partial restore: only restore a.txt to v1
	output, err := h.RunRestore("v1", "a.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Restored to v1")

	// a.txt should be v1 content
	if got := h.ReadFile("a.txt"); got != "v1 a" {
		t.Errorf("a.txt = %q, want %q", got, "v1 a")
	}
	// b.txt should remain v2 content (untouched)
	if got := h.ReadFile("b.txt"); got != "v2 b" {
		t.Errorf("b.txt = %q, want %q (should be untouched)", got, "v2 b")
	}
}

// TC-RESTORE-009: Partial restore of a directory subtree
func TestRestore_Directory(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// v1: src/main.go, src/lib/helper.go, README.md
	h.WriteFile("src/main.go", "v1 main")
	h.WriteFile("src/lib/helper.go", "v1 helper")
	h.WriteFile("README.md", "v1 readme")
	h.AddAndSave([]string{"src/main.go", "src/lib/helper.go", "README.md"}, "v1")

	// v2: modify all files
	h.WriteFile("src/main.go", "v2 main")
	h.WriteFile("src/lib/helper.go", "v2 helper")
	h.WriteFile("README.md", "v2 readme")
	h.AddAndSave([]string{"src/main.go", "src/lib/helper.go", "README.md"}, "v2")

	// Partial restore: only restore src/ to v1
	output, err := h.RunRestore("v1", "src/")
	h.AssertNoError(err)
	h.AssertContains(output, "Restored to v1")

	// src/ files should be v1 content
	if got := h.ReadFile("src/main.go"); got != "v1 main" {
		t.Errorf("src/main.go = %q, want %q", got, "v1 main")
	}
	if got := h.ReadFile("src/lib/helper.go"); got != "v1 helper" {
		t.Errorf("src/lib/helper.go = %q, want %q", got, "v1 helper")
	}
	// README.md should remain v2 content (untouched)
	if got := h.ReadFile("README.md"); got != "v2 readme" {
		t.Errorf("README.md = %q, want %q (should be untouched)", got, "v2 readme")
	}
}

// TC-RESTORE-010: Partial restore deletes files added after target version
func TestRestore_PartialDelete(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// v1: src/main.go
	h.WriteFile("src/main.go", "v1 main")
	h.AddAndSave([]string{"src/main.go"}, "v1")

	// v2: add src/util.go
	h.WriteFile("src/util.go", "v2 util")
	h.AddAndSave([]string{"src/util.go"}, "v2")

	// Partial restore: restore src/ to v1
	// src/util.go should be deleted (not in v1)
	output, err := h.RunRestore("v1", "src/")
	h.AssertNoError(err)
	h.AssertContains(output, "Restored to v1")
	h.AssertContains(output, "1 deleted")

	// src/main.go should be v1 content
	if got := h.ReadFile("src/main.go"); got != "v1 main" {
		t.Errorf("src/main.go = %q, want %q", got, "v1 main")
	}
	// src/util.go should be deleted
	if h.FileExists("src/util.go") {
		t.Error("src/util.go should be deleted after partial restore to v1")
	}
}

// TC-RESTORE-011: Partial restore does not touch files outside the filter
func TestRestore_PartialPreservesOutside(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// v1: a.txt, b.txt
	h.WriteFile("a.txt", "v1 a")
	h.WriteFile("b.txt", "v1 b")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v1")

	// v2: modify a.txt, delete b.txt, add c.txt
	h.WriteFile("a.txt", "v2 a")
	h.WriteFile("c.txt", "v2 c")
	h.AddAndSave([]string{"a.txt", "c.txt"}, "v2")

	// Partial restore: only restore a.txt to v1
	_, err := h.RunRestore("v1", "a.txt")
	h.AssertNoError(err)

	// a.txt should be v1 content
	if got := h.ReadFile("a.txt"); got != "v1 a" {
		t.Errorf("a.txt = %q, want %q", got, "v1 a")
	}
	// c.txt should still exist (not in filter, not touched)
	if !h.FileExists("c.txt") {
		t.Error("c.txt should be preserved (outside filter)")
	}
	if got := h.ReadFile("c.txt"); got != "v2 c" {
		t.Errorf("c.txt = %q, want %q", got, "v2 c")
	}
}

// TC-RESTORE-012: Partial restore with nonexistent path errors
func TestRestore_PartialPathNotFound(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "v1 a")
	h.AddAndSave([]string{"a.txt"}, "v1")

	_, err := h.RunRestore("v1", "nonexistent.txt")
	h.AssertError(err)
}

// TC-RESTORE-013: Partial restore with multiple paths
func TestRestore_MultiplePaths(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// v1: a.txt, b.txt, c.txt
	h.WriteFile("a.txt", "v1 a")
	h.WriteFile("b.txt", "v1 b")
	h.WriteFile("c.txt", "v1 c")
	h.AddAndSave([]string{"a.txt", "b.txt", "c.txt"}, "v1")

	// v2: modify all
	h.WriteFile("a.txt", "v2 a")
	h.WriteFile("b.txt", "v2 b")
	h.WriteFile("c.txt", "v2 c")
	h.AddAndSave([]string{"a.txt", "b.txt", "c.txt"}, "v2")

	// Partial restore: restore a.txt and c.txt to v1
	output, err := h.RunRestore("v1", "a.txt", "c.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Restored to v1")
	h.AssertContains(output, "2 modified")

	// a.txt and c.txt should be v1 content
	if got := h.ReadFile("a.txt"); got != "v1 a" {
		t.Errorf("a.txt = %q, want %q", got, "v1 a")
	}
	if got := h.ReadFile("c.txt"); got != "v1 c" {
		t.Errorf("c.txt = %q, want %q", got, "v1 c")
	}
	// b.txt should remain v2 content (untouched)
	if got := h.ReadFile("b.txt"); got != "v2 b" {
		t.Errorf("b.txt = %q, want %q (should be untouched)", got, "v2 b")
	}
}

// TC-RESTORE-014: Partial restore only checks dirty state for filtered paths
func TestRestore_PartialDirtyCheckScoped(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// v1: a.txt, b.txt
	h.WriteFile("a.txt", "v1 a")
	h.WriteFile("b.txt", "v1 b")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v1")

	// v2: modify both
	h.WriteFile("a.txt", "v2 a")
	h.WriteFile("b.txt", "v2 b")
	h.AddAndSave([]string{"a.txt", "b.txt"}, "v2")

	// Stage a change to b.txt (but NOT to a.txt)
	h.WriteFile("b.txt", "staged b")
	_, err := h.RunAdd("b.txt")
	h.AssertNoError(err)

	// Partial restore of a.txt should succeed without --force
	// because b.txt's staged changes are outside the filter.
	output, err := h.RunRestore("v1", "a.txt")
	h.AssertNoError(err)
	h.AssertContains(output, "Restored to v1")

	// a.txt should be v1 content
	if got := h.ReadFile("a.txt"); got != "v1 a" {
		t.Errorf("a.txt = %q, want %q", got, "v1 a")
	}
}
