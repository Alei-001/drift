package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCheck_ValidRepo(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "file.txt", "content", "snap")

	out := captureStdout(t, func() {
		if err := runCmd(checkCmd, nil); err != nil {
			t.Fatalf("check: %v", err)
		}
	})

	if !strings.Contains(out, "Check") {
		t.Errorf("stdout = %q, want 'Check'", out)
	}
	if !strings.Contains(out, "blocks passed") {
		t.Errorf("stdout = %q, want 'blocks passed'", out)
	}
}

func TestCheck_EmptyRepo(t *testing.T) {
	setupTestRepo(t)

	out := captureStdout(t, func() {
		if err := runCmd(checkCmd, nil); err != nil {
			t.Fatalf("check: %v", err)
		}
	})

	if !strings.Contains(out, "0 blocks passed") {
		t.Errorf("stdout = %q, want '0 blocks passed'", out)
	}
}

func TestCheck_JSON(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "f.txt", "data", "s")

	globalJSON = true
	out := captureStdout(t, func() {
		if err := runCmd(checkCmd, nil); err != nil {
			t.Fatalf("check: %v", err)
		}
	})

	var env JSONEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if env.Command != "check" || env.Status != "ok" {
		t.Errorf("envelope = %+v, want command=check status=ok", env)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}
	if data["corrupt"].(float64) != 0 {
		t.Errorf("data.corrupt = %v, want 0", data["corrupt"])
	}
	if data["missing"].(float64) != 0 {
		t.Errorf("data.missing = %v, want 0", data["missing"])
	}
}

func TestCheck_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(checkCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not a drift repository") {
		t.Errorf("stderr = %q, want 'not a drift repository'", strings.TrimSpace(errOut))
	}
}
