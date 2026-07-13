package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_NewRepo(t *testing.T) {
	workDir := setupEmptyDir(t)

	out := captureStdout(t, func() {
		if err := runCmd(initCmd, nil); err != nil {
			t.Fatalf("init: %v", err)
		}
	})

	if _, err := os.Stat(filepath.Join(workDir, ".drift")); err != nil {
		t.Errorf(".drift directory not created: %v", err)
	}
	if !strings.Contains(out, "Initialized") {
		t.Errorf("stdout = %q, want 'Initialized'", out)
	}
	if !strings.Contains(out, workDir) {
		t.Errorf("stdout = %q, want path %q", out, workDir)
	}
}

func TestInit_ExistingDir(t *testing.T) {
	workDir := setupTestRepo(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(initCmd, nil); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "already a drift repository") {
		t.Errorf("stderr = %q, want 'already a drift repository'", errOut)
	}
	// Ensure the original repo is untouched (still has .drift/).
	if _, err := os.Stat(filepath.Join(workDir, ".drift")); err != nil {
		t.Errorf(".drift directory missing after re-init: %v", err)
	}
}

func TestInit_JSON(t *testing.T) {
	setupEmptyDir(t)
	globalJSON = true

	out := captureStdout(t, func() {
		if err := runCmd(initCmd, nil); err != nil {
			t.Fatalf("init: %v", err)
		}
	})

	var env JSONEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if env.Command != "init" || env.Status != "ok" {
		t.Errorf("envelope = %+v, want command=init status=ok", env)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}
	path, _ := data["path"].(string)
	if !strings.HasSuffix(path, ".drift") {
		t.Errorf("data.path = %q, want suffix '.drift'", path)
	}
}
