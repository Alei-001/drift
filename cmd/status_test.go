package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestStatus_EmptyRepo(t *testing.T) {
	setupTestRepo(t)

	out := captureStdout(t, func() {
		if err := runCmd(statusCmd, nil); err != nil {
			t.Fatalf("status: %v", err)
		}
	})

	if !strings.Contains(out, "Status") {
		t.Errorf("stdout = %q, want 'Status'", out)
	}
	if !strings.Contains(out, "Nothing changed") {
		t.Errorf("stdout = %q, want 'Nothing changed'", out)
	}
}

func TestStatus_WithChanges(t *testing.T) {
	workDir := setupTestRepo(t)
	writeFile(t, workDir, "new.txt", "new content")

	out := captureStdout(t, func() {
		if err := runCmd(statusCmd, nil); err != nil {
			t.Fatalf("status: %v", err)
		}
	})

	if !strings.Contains(out, "new.txt") {
		t.Errorf("stdout = %q, want 'new.txt'", out)
	}
	if !strings.Contains(out, "+") {
		t.Errorf("stdout = %q, want '+' marker for added file", out)
	}
}

func TestStatus_JSON(t *testing.T) {
	workDir := setupTestRepo(t)
	writeFile(t, workDir, "file.txt", "content")

	globalJSON = true
	out := captureStdout(t, func() {
		if err := runCmd(statusCmd, nil); err != nil {
			t.Fatalf("status: %v", err)
		}
	})

	var env JSONEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if env.Command != "status" || env.Status != "ok" {
		t.Errorf("envelope = %+v, want command=status status=ok", env)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}
	added, _ := data["added"].([]interface{})
	if len(added) != 1 {
		t.Errorf("data.added = %v, want 1 entry", added)
	}
	summary, _ := data["summary"].(map[string]interface{})
	if summary["total"].(float64) != 1 {
		t.Errorf("summary.total = %v, want 1", summary["total"])
	}
}

func TestStatus_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(statusCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not a drift repository") {
		t.Errorf("stderr = %q, want 'not a drift repository'", strings.TrimSpace(errOut))
	}
}
