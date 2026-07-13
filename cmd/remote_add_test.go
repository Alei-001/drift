package cmd

import (
	"errors"
	"strings"
	"testing"
)

// resetRemoteAddFlags resets the cobra-level flags on remoteAddCmd so tests
// don't leak flag state into each other. resetFlags() only handles package-
// level vars, not the command's own flag set.
func resetRemoteAddFlags(t *testing.T) {
	t.Helper()
	for _, name := range []string{"type", "url", "user", "password"} {
		if err := remoteAddCmd.Flags().Set(name, ""); err != nil {
			t.Fatalf("reset %s flag: %v", name, err)
		}
	}
	if err := remoteAddCmd.Flags().Set("type", "webdav"); err != nil {
		t.Fatalf("reset type flag: %v", err)
	}
	for _, name := range []string{"password-stdin", "no-save-password"} {
		if err := remoteAddCmd.Flags().Set(name, "false"); err != nil {
			t.Fatalf("reset %s flag: %v", name, err)
		}
	}
}

func TestRemoteAdd_InteractiveNoTerminal(t *testing.T) {
	setupTestRepo(t)
	resetRemoteAddFlags(t)

	// With url and user empty, the command enters interactive mode, but
	// test stdin is not a terminal — should fail with ErrSilent.
	errOut := captureStderr(t, func() {
		if err := runCmd(remoteAddCmd, []string{"origin"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "interactive mode requires a terminal") {
		t.Errorf("stderr = %q, want 'interactive mode requires a terminal'", strings.TrimSpace(errOut))
	}
}

func TestRemoteAdd_Success(t *testing.T) {
	setupTestRepo(t)
	resetRemoteAddFlags(t)

	if err := remoteAddCmd.Flags().Set("url", "http://example.com/dav"); err != nil {
		t.Fatalf("set url flag: %v", err)
	}
	if err := remoteAddCmd.Flags().Set("user", "testuser"); err != nil {
		t.Fatalf("set user flag: %v", err)
	}
	if err := remoteAddCmd.Flags().Set("no-save-password", "true"); err != nil {
		t.Fatalf("set no-save-password flag: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runCmd(remoteAddCmd, []string{"origin"}); err != nil {
			t.Fatalf("remote add: %v", err)
		}
	})

	if !strings.Contains(out, "origin") {
		t.Errorf("stdout = %q, want 'origin'", out)
	}

	// Verify the remote shows up in the list.
	listOut := captureStdout(t, func() {
		if err := runCmd(remoteListCmd, nil); err != nil {
			t.Fatalf("remote list: %v", err)
		}
	})
	if !strings.Contains(listOut, "origin") {
		t.Errorf("remote list = %q, want 'origin'", listOut)
	}
}

func TestRemoteAdd_DuplicateName(t *testing.T) {
	setupTestRepo(t)
	resetRemoteAddFlags(t)

	// Configure flags for the first add.
	if err := remoteAddCmd.Flags().Set("url", "http://example.com/dav"); err != nil {
		t.Fatalf("set url flag: %v", err)
	}
	if err := remoteAddCmd.Flags().Set("user", "testuser"); err != nil {
		t.Fatalf("set user flag: %v", err)
	}
	if err := remoteAddCmd.Flags().Set("no-save-password", "true"); err != nil {
		t.Fatalf("set no-save-password flag: %v", err)
	}

	// Add origin once.
	_ = captureStdout(t, func() {
		if err := runCmd(remoteAddCmd, []string{"origin"}); err != nil {
			t.Fatalf("first remote add: %v", err)
		}
	})

	// Add origin again — should fail.
	errOut := captureStderr(t, func() {
		if err := runCmd(remoteAddCmd, []string{"origin"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent for duplicate remote, got %v", err)
		}
	})

	if !strings.Contains(errOut, "already exists") {
		t.Errorf("stderr = %q, want 'already exists'", strings.TrimSpace(errOut))
	}
}
