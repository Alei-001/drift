package cmd

import (
	"errors"
	"strings"
	"testing"
)

func TestLsRemote_NoRemote(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "f.txt", "data", "initial")

	errOut := captureStderr(t, func() {
		if err := runCmd(lsRemoteCmd, []string{"origin"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "could not list remote refs") {
		t.Errorf("stderr = %q, want 'could not list remote refs'", strings.TrimSpace(errOut))
	}
}

func TestLsRemote_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(lsRemoteCmd, []string{"origin"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	// ls-remote resolves remotes via remotes.json under .drift/, so a
	// missing .drift directory surfaces as a "could not list remote refs"
	// error (loadRemotesOrReport returns "not a drift repository").
	if !strings.Contains(errOut, "could not list remote refs") {
		t.Errorf("stderr = %q, want 'could not list remote refs'", strings.TrimSpace(errOut))
	}
}
