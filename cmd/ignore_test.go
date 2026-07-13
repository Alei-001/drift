package cmd

import (
	"errors"
	"strings"
	"testing"
)

func TestIgnore_AddAndList(t *testing.T) {
	setupTestRepo(t)

	// Add some patterns.
	_ = captureStdout(t, func() {
		if err := runCmd(ignoreAddCmd, []string{"*.tmp", "build/"}); err != nil {
			t.Fatalf("ignore add: %v", err)
		}
	})

	// List should show both patterns.
	out := captureStdout(t, func() {
		if err := runCmd(ignoreListCmd, nil); err != nil {
			t.Fatalf("ignore list: %v", err)
		}
	})

	if !strings.Contains(out, "*.tmp") {
		t.Errorf("list = %q, want '*.tmp'", out)
	}
	if !strings.Contains(out, "build/") {
		t.Errorf("list = %q, want 'build/'", out)
	}
}

func TestIgnore_Remove(t *testing.T) {
	setupTestRepo(t)

	// Add a pattern.
	_ = captureStdout(t, func() {
		if err := runCmd(ignoreAddCmd, []string{"*.log"}); err != nil {
			t.Fatalf("ignore add: %v", err)
		}
	})

	// Remove it.
	out := captureStdout(t, func() {
		if err := runCmd(ignoreRemoveCmd, []string{"*.log"}); err != nil {
			t.Fatalf("ignore remove: %v", err)
		}
	})

	if !strings.Contains(out, "removed") {
		t.Errorf("remove stdout = %q, want 'removed'", out)
	}

	// List should not contain the pattern.
	listOut := captureStdout(t, func() {
		if err := runCmd(ignoreListCmd, nil); err != nil {
			t.Fatalf("ignore list: %v", err)
		}
	})
	if strings.Contains(listOut, "*.log") {
		t.Errorf("list = %q, should not contain '*.log'", listOut)
	}
}

func TestIgnore_RemoveNonexistent(t *testing.T) {
	setupTestRepo(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(ignoreRemoveCmd, []string{"nonexistent"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not found") {
		t.Errorf("stderr = %q, want 'not found'", strings.TrimSpace(errOut))
	}
}

func TestIgnore_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(ignoreListCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not a drift repository") {
		t.Errorf("stderr = %q, want 'not a drift repository'", strings.TrimSpace(errOut))
	}
}
