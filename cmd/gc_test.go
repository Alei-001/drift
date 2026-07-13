package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestGC_NoUnreachable(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "file.txt", "content", "snap")

	out := captureStdout(t, func() {
		if err := runCmd(gcCmd, nil); err != nil {
			t.Fatalf("gc: %v", err)
		}
	})

	if !strings.Contains(out, "GC") {
		t.Errorf("stdout = %q, want 'GC'", out)
	}
	if !strings.Contains(out, "nothing to reclaim") {
		t.Errorf("stdout = %q, want 'nothing to reclaim'", out)
	}
}

func TestGC_DryRun(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "a.txt", "a", "first")
	saveSnapshot(t, workDir, "b.txt", "b", "second")

	// Undo the last save to make it unreachable, then gc --dry-run.
	_ = captureStdout(t, func() {
		if err := runCmd(undoCmd, nil); err != nil {
			t.Fatalf("undo: %v", err)
		}
	})

	gcDryRun = true
	out := captureStdout(t, func() {
		if err := runCmd(gcCmd, nil); err != nil {
			t.Fatalf("gc: %v", err)
		}
	})

	if !strings.Contains(out, "dry-run") {
		t.Errorf("stdout = %q, want 'dry-run'", out)
	}
}

func TestGC_JSON(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "f.txt", "data", "s")

	globalJSON = true
	out := captureStdout(t, func() {
		if err := runCmd(gcCmd, nil); err != nil {
			t.Fatalf("gc: %v", err)
		}
	})

	var env JSONEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if env.Command != "gc" || env.Status != "ok" {
		t.Errorf("envelope = %+v, want command=gc status=ok", env)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}
	if data["snapshots_removed"].(float64) != 0 {
		t.Errorf("data.snapshots_removed = %v, want 0", data["snapshots_removed"])
	}
}

func TestGC_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(gcCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not a drift repository") {
		t.Errorf("stderr = %q, want 'not a drift repository'", strings.TrimSpace(errOut))
	}
}
