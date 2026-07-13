package cmd

import (
	"errors"
	"strings"
	"testing"
)

func TestConfig_List(t *testing.T) {
	setupTestRepo(t)

	out := captureStdout(t, func() {
		if err := runCmd(configListCmd, nil); err != nil {
			t.Fatalf("config list: %v", err)
		}
	})

	if !strings.Contains(out, "user.name") {
		t.Errorf("stdout = %q, want 'user.name'", out)
	}
	if !strings.Contains(out, "user.email") {
		t.Errorf("stdout = %q, want 'user.email'", out)
	}
}

func TestConfig_SetAndGet(t *testing.T) {
	setupTestRepo(t)

	// Set user.name.
	_ = captureStdout(t, func() {
		if err := runCmd(configSetCmd, []string{"user.name", "Test User"}); err != nil {
			t.Fatalf("config set: %v", err)
		}
	})

	// Get user.name and verify.
	out := captureStdout(t, func() {
		if err := runCmd(configGetCmd, []string{"user.name"}); err != nil {
			t.Fatalf("config get: %v", err)
		}
	})

	if !strings.Contains(out, "Test User") {
		t.Errorf("stdout = %q, want 'Test User'", out)
	}
}

func TestConfig_UnknownKey(t *testing.T) {
	setupTestRepo(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(configGetCmd, []string{"unknown.key"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "unknown config key") {
		t.Errorf("stderr = %q, want 'unknown config key'", strings.TrimSpace(errOut))
	}
}

func TestConfig_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(configListCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not a drift repository") {
		t.Errorf("stderr = %q, want 'not a drift repository'", strings.TrimSpace(errOut))
	}
}
