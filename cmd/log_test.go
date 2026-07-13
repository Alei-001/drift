package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestLog_EmptyRepo(t *testing.T) {
	setupTestRepo(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(logCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "no snapshots yet") {
		t.Errorf("stderr = %q, want 'no snapshots yet'", strings.TrimSpace(errOut))
	}
}

func TestLog_WithSnapshots(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "first.txt", "content1", "first snapshot")
	saveSnapshot(t, workDir, "second.txt", "content2", "second snapshot")

	out := captureStdout(t, func() {
		if err := runCmd(logCmd, nil); err != nil {
			t.Fatalf("log: %v", err)
		}
	})

	if !strings.Contains(out, "History") {
		t.Errorf("stdout = %q, want 'History'", out)
	}
	if !strings.Contains(out, "first snapshot") {
		t.Errorf("stdout = %q, want 'first snapshot'", out)
	}
	if !strings.Contains(out, "second snapshot") {
		t.Errorf("stdout = %q, want 'second snapshot'", out)
	}
}

func TestLog_JSON(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "a.txt", "a", "snap a")

	globalJSON = true
	out := captureStdout(t, func() {
		if err := runCmd(logCmd, nil); err != nil {
			t.Fatalf("log: %v", err)
		}
	})

	var env JSONEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if env.Command != "log" || env.Status != "ok" {
		t.Errorf("envelope = %+v, want command=log status=ok", env)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}
	snapshots, _ := data["snapshots"].([]interface{})
	if len(snapshots) != 1 {
		t.Errorf("data.snapshots has %d entries, want 1", len(snapshots))
	}
}

func TestLog_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(logCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not a drift repository") {
		t.Errorf("stderr = %q, want 'not a drift repository'", strings.TrimSpace(errOut))
	}
}
