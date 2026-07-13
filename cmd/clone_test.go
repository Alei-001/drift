package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestClone_InvalidURL(t *testing.T) {
	setupTestRepo(t)

	// Set the --type flag on cloneCmd via Flags().Set so RunE can read it.
	if err := cloneCmd.Flags().Set("type", "webdav"); err != nil {
		t.Fatalf("set type flag: %v", err)
	}

	errOut := captureStderr(t, func() {
		if err := runCmd(cloneCmd, []string{"http://invalid.invalid/nonexistent"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "clone failed") {
		t.Errorf("stderr = %q, want 'clone failed'", strings.TrimSpace(errOut))
	}
}

func TestClone_JSON(t *testing.T) {
	setupTestRepo(t)
	globalJSON = true

	if err := cloneCmd.Flags().Set("type", "webdav"); err != nil {
		t.Fatalf("set type flag: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runCmd(cloneCmd, []string{"http://invalid.invalid/nonexistent"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	var env JSONEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if env.Command != "clone" || env.Status != "failed" {
		t.Errorf("envelope = %+v, want command=clone status=failed", env)
	}
}

func TestClone_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	// clone doesn't require a repo (it creates one), but it still needs a
	// valid URL. An invalid URL should fail with "clone failed".
	if err := cloneCmd.Flags().Set("type", "webdav"); err != nil {
		t.Fatalf("set type flag: %v", err)
	}

	errOut := captureStderr(t, func() {
		if err := runCmd(cloneCmd, []string{"http://invalid.invalid/nonexistent"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "clone failed") {
		t.Errorf("stderr = %q, want 'clone failed'", strings.TrimSpace(errOut))
	}
}
