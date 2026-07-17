package cmd

import (
	"github.com/Alei-001/drift/internal/project"
	"errors"
	"strings"
	"testing"
)

func TestWatch_StatusInactive(t *testing.T) {
	setupTestRepo(t)

	// No daemon running — status should report inactive.
	out := captureStdout(t, func() {
		if err := runCmd(watchStatusCmd, nil); err != nil {
			t.Fatalf("watch status: %v", err)
		}
	})

	if !strings.Contains(out, "inactive") {
		t.Errorf("stdout = %q, want 'inactive'", out)
	}
}

func TestWatch_OnInvalidInterval(t *testing.T) {
	setupTestRepo(t)

	// Interval must be positive; zero should fail.
	watchInterval = 0
	t.Cleanup(func() { watchInterval = 0 })

	errOut := captureStderr(t, func() {
		if err := runCmd(watchOnCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "interval") {
		t.Errorf("stderr = %q, want 'interval'", strings.TrimSpace(errOut))
	}
}

func TestWatch_OffNoDaemon(t *testing.T) {
	setupTestRepo(t)

	// No daemon running — off should fail.
	errOut := captureStderr(t, func() {
		if err := runCmd(watchOffCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	// The error message varies by platform; just check it's non-empty.
	if strings.TrimSpace(errOut) == "" {
		t.Error("expected non-empty stderr for watch off with no daemon")
	}
}

func TestWatch_OnNotARepo(t *testing.T) {
	setupEmptyDir(t)

	// watchOnCmd validates the interval first, then opens the project. Set
	// a valid interval so the project-open check is reached.
	watchInterval = 60
	t.Cleanup(func() { watchInterval = 0 })

	errOut := captureStderr(t, func() {
		if err := runCmd(watchOnCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not a drift repository") {
		t.Errorf("stderr = %q, want 'not a drift repository'", strings.TrimSpace(errOut))
	}
}
