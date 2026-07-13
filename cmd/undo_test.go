package cmd

import (
	"errors"
	"strings"
	"testing"
)

func TestUndo_NoHistory(t *testing.T) {
	setupTestRepo(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(undoCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "no snapshot to undo") {
		t.Errorf("stderr = %q, want 'no snapshot to undo'", strings.TrimSpace(errOut))
	}
}

func TestUndo_Success(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "first.txt", "content1", "first snapshot")
	saveSnapshot(t, workDir, "second.txt", "content2", "second snapshot")

	out := captureStdout(t, func() {
		if err := runCmd(undoCmd, nil); err != nil {
			t.Fatalf("undo: %v", err)
		}
	})

	if !strings.Contains(out, "Undone") {
		t.Errorf("stdout = %q, want 'Undone'", out)
	}
	if !strings.Contains(out, "second snapshot") {
		t.Errorf("stdout = %q, want 'second snapshot' as removed", out)
	}
	if !strings.Contains(out, "first snapshot") {
		t.Errorf("stdout = %q, want 'first snapshot' as new HEAD", out)
	}

	// Verify log only shows the first snapshot now.
	logOut := captureStdout(t, func() {
		if err := runCmd(logCmd, nil); err != nil {
			t.Fatalf("log: %v", err)
		}
	})
	if strings.Contains(logOut, "second snapshot") {
		t.Errorf("log = %q, want 'second snapshot' to be gone after undo", logOut)
	}
	if !strings.Contains(logOut, "first snapshot") {
		t.Errorf("log = %q, want 'first snapshot' to still be present", logOut)
	}
}

func TestUndo_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(undoCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not a drift repository") {
		t.Errorf("stderr = %q, want 'not a drift repository'", strings.TrimSpace(errOut))
	}
}
