package cmd

import (
	"errors"
	"strings"
	"testing"
)

func TestPull_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(pullCmd, []string{"origin"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not a drift repository") {
		t.Errorf("stderr = %q, want 'not a drift repository'", strings.TrimSpace(errOut))
	}
}

func TestPull_NoRemote(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "f.txt", "data", "initial")

	errOut := captureStderr(t, func() {
		if err := runCmd(pullCmd, []string{"origin"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "pull failed") {
		t.Errorf("stderr = %q, want 'pull failed'", strings.TrimSpace(errOut))
	}
}

func TestPull_JSON(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "f.txt", "data", "initial")

	globalJSON = true
	_ = captureStdout(t, func() {
		if err := runCmd(pullCmd, []string{"origin"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})
}
