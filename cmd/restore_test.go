package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestore_InvalidSnapshot(t *testing.T) {
	setupTestRepo(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(restoreCmd, []string{"id:nonexistent"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not found") {
		t.Errorf("stderr = %q, want 'not found'", strings.TrimSpace(errOut))
	}
}

func TestRestore_ValidSnapshot(t *testing.T) {
	workDir := setupTestRepo(t)
	sid := saveSnapshot(t, workDir, "f.txt", "original", "initial")

	// Modify the file so restore has something to do.
	writeFile(t, workDir, "f.txt", "modified")

	out := captureStdout(t, func() {
		if err := runCmd(restoreCmd, []string{"id:" + sid}); err != nil {
			t.Fatalf("restore: %v", err)
		}
	})

	if !strings.Contains(out, "Restored") {
		t.Errorf("stdout = %q, want 'Restored'", out)
	}

	// Verify the file content was restored.
	content, err := readFileContent(workDir, "f.txt")
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if content != "original" {
		t.Errorf("file content = %q, want 'original'", content)
	}
}

func TestRestore_NoBackupOnFullRestore(t *testing.T) {
	workDir := setupTestRepo(t)
	sid := saveSnapshot(t, workDir, "f.txt", "original", "initial")

	restoreNoBackup = true
	errOut := captureStderr(t, func() {
		if err := runCmd(restoreCmd, []string{"id:" + sid}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent for --no-backup on full restore, got %v", err)
		}
	})

	if !strings.Contains(errOut, "--no-backup is only allowed") {
		t.Errorf("stderr = %q, want '--no-backup is only allowed'", strings.TrimSpace(errOut))
	}
}

func TestRestore_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(restoreCmd, []string{"id:abc"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not a drift repository") {
		t.Errorf("stderr = %q, want 'not a drift repository'", strings.TrimSpace(errOut))
	}
}

// readFileContent reads a file under workDir and returns its content as a string.
func readFileContent(workDir, relPath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(workDir, relPath))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
