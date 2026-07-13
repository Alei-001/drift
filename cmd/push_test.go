package cmd

import (
	"errors"
	"strings"
	"testing"
)

func TestPush_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(pushCmd, []string{"origin"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not a drift repository") {
		t.Errorf("stderr = %q, want 'not a drift repository'", strings.TrimSpace(errOut))
	}
}

func TestPush_NoRemote(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "f.txt", "data", "initial")

	errOut := captureStderr(t, func() {
		if err := runCmd(pushCmd, []string{"origin"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	// The push command should fail because the remote 'origin' is not
	// configured. The error is reported as "push failed".
	if !strings.Contains(errOut, "push failed") {
		t.Errorf("stderr = %q, want 'push failed'", strings.TrimSpace(errOut))
	}
}

func TestPush_JSON(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "f.txt", "data", "initial")

	globalJSON = true
	_ = captureStdout(t, func() {
		if err := runCmd(pushCmd, []string{"origin"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})
	// In JSON mode, the failure envelope is written to stdout. We don't
	// assert its contents here because the exact error depends on remote
	// resolution; the key is that it returns ErrSilent (already reported).
}
