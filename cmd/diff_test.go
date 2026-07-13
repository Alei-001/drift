package cmd

import (
	"errors"
	"strings"
	"testing"
)

func TestDiff_NoSnapshot(t *testing.T) {
	setupTestRepo(t)

	// Empty repo with no snapshots — diff has nothing to compare against.
	errOut := captureStderr(t, func() {
		if err := runCmd(diffCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "no snapshot to compare against") {
		t.Errorf("stderr = %q, want 'no snapshot to compare against'", strings.TrimSpace(errOut))
	}
}

func TestDiff_WithChanges(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "file.txt", "original content", "initial")

	// Modify the file so the workspace differs from HEAD.
	writeFile(t, workDir, "file.txt", "modified content")

	out := captureStdout(t, func() {
		if err := runCmd(diffCmd, nil); err != nil {
			t.Fatalf("diff: %v", err)
		}
	})

	if !strings.Contains(out, "file.txt") {
		t.Errorf("stdout = %q, want 'file.txt'", out)
	}
	if !strings.Contains(out, "~") {
		t.Errorf("stdout = %q, want '~' (modified marker)", out)
	}
}

func TestDiff_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(diffCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not a drift repository") {
		t.Errorf("stderr = %q, want 'not a drift repository'", strings.TrimSpace(errOut))
	}
}
