package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSave_NoChanges(t *testing.T) {
	setupTestRepo(t)

	errOut := captureStderr(t, func() {
		saveMessage = "test"
		if err := runCmd(saveCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "nothing to save") {
		t.Errorf("stderr = %q, want 'nothing to save'", strings.TrimSpace(errOut))
	}
}

func TestSave_NewFile(t *testing.T) {
	workDir := setupTestRepo(t)
	writeFile(t, workDir, "hello.txt", "Hello World")

	saveMessage = "first save"
	out := captureStdout(t, func() {
		if err := runCmd(saveCmd, nil); err != nil {
			t.Fatalf("save: %v", err)
		}
	})

	if !strings.Contains(out, "Saved") {
		t.Errorf("stdout = %q, want 'Saved'", out)
	}
	if !strings.Contains(out, "hello.txt") {
		t.Errorf("stdout = %q, want 'hello.txt'", out)
	}
	if !strings.Contains(out, "first save") {
		t.Errorf("stdout = %q, want message 'first save'", out)
	}

	// Verify the snapshot was persisted: .drift/snapshots/ should be non-empty.
	entries, err := os.ReadDir(filepath.Join(workDir, ".drift", "snapshots"))
	if err != nil {
		t.Fatalf("read snapshots dir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("no snapshots written to .drift/snapshots/")
	}
}

func TestSave_JSON(t *testing.T) {
	workDir := setupTestRepo(t)
	writeFile(t, workDir, "data.txt", "some content")

	globalJSON = true
	saveMessage = "json save"
	out := captureStdout(t, func() {
		if err := runCmd(saveCmd, nil); err != nil {
			t.Fatalf("save: %v", err)
		}
	})

	var env JSONEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if env.Command != "save" || env.Status != "ok" {
		t.Errorf("envelope = %+v, want command=save status=ok", env)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}
	if data["message"] != "json save" {
		t.Errorf("data.message = %v, want 'json save'", data["message"])
	}
	id, _ := data["id"].(string)
	if id == "" {
		t.Error("data.id is empty")
	}
}

func TestSave_Locked(t *testing.T) {
	workDir := setupTestRepo(t)
	writeFile(t, workDir, "a.txt", "a")

	// Write a non-stale workspace lock: PID 0 skips the process-exists check,
	// and a fresh timestamp keeps it within the stale-timeout window.
	lockPath := filepath.Join(workDir, ".drift", "workspace.lock")
	lockData := fmt.Sprintf(`{"pid":0,"timestamp":%d}`, time.Now().Unix())
	if err := os.WriteFile(lockPath, []byte(lockData), 0644); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	saveMessage = "should fail"
	err := runCmd(saveCmd, nil)
	// The save command may return ErrSilent (lock reported) or a wrapped
	// porcelain.ErrLocked. Either way it must be non-nil.
	if err == nil {
		t.Fatal("expected error from save on locked workspace, got nil")
	}
}
