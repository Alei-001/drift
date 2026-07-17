package cmd

import (
	"github.com/Alei-001/drift/internal/branch"
	"errors"
	"strings"
	"testing"
)

func TestImport_NonexistentBranch(t *testing.T) {
	setupTestRepo(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(importCmd, []string{"nonexistent-branch", "file.txt"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "import failed") {
		t.Errorf("stderr = %q, want 'import failed'", strings.TrimSpace(errOut))
	}
}

func TestImport_FromBranch(t *testing.T) {
	workDir := setupTestRepo(t)
	// Create a snapshot with a file on main.
	saveSnapshot(t, workDir, "shared.txt", "branch content", "initial")

	// Create a branch pointing at HEAD.
	_ = captureStdout(t, func() {
		if err := runCmd(branchCreateCmd, []string{"feature"}); err != nil {
			t.Fatalf("branch create: %v", err)
		}
	})

	// Overwrite the file in the workspace so the import actually changes it.
	writeFile(t, workDir, "shared.txt", "workspace content")

	// Import the file from the feature branch.
	out := captureStdout(t, func() {
		if err := runCmd(importCmd, []string{"feature", "shared.txt"}); err != nil {
			t.Fatalf("import: %v", err)
		}
	})

	if !strings.Contains(out, "Imported") {
		t.Errorf("stdout = %q, want 'Imported'", out)
	}

	// The file should now contain the branch's content.
	content, err := readFileContent(workDir, "shared.txt")
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if content != "branch content" {
		t.Errorf("file content = %q, want 'branch content'", content)
	}
}

func TestImport_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(importCmd, []string{"main", "file.txt"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not a drift repository") {
		t.Errorf("stderr = %q, want 'not a drift repository'", strings.TrimSpace(errOut))
	}
}
