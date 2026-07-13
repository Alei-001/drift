package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSwitch_NonexistentBranch(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "f.txt", "data", "initial")

	errOut := captureStderr(t, func() {
		if err := runCmd(switchCmd, []string{"nonexistent"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not found") {
		t.Errorf("stderr = %q, want 'not found'", strings.TrimSpace(errOut))
	}
}

func TestSwitch_CreateAndSwitch(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "f.txt", "data", "initial")

	switchCreate = true
	out := captureStdout(t, func() {
		if err := runCmd(switchCmd, []string{"feature"}); err != nil {
			t.Fatalf("switch -c: %v", err)
		}
	})

	if !strings.Contains(out, "Switched to 'feature'") {
		t.Errorf("stdout = %q, want 'Switched to 'feature''", out)
	}

	// Verify the current branch is now 'feature'.
	listOut := captureStdout(t, func() {
		if err := runCmd(branchListCmd, nil); err != nil {
			t.Fatalf("branch list: %v", err)
		}
	})
	lines := strings.Split(strings.TrimSpace(listOut), "\n")
	found := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "*") && strings.Contains(line, "feature") {
			found = true
		}
	}
	if !found {
		t.Errorf("branch list = %q, want 'feature' as current branch", listOut)
	}
}

func TestSwitch_JSON(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "f.txt", "data", "initial")

	switchCreate = true
	globalJSON = true
	out := captureStdout(t, func() {
		if err := runCmd(switchCmd, []string{"dev"}); err != nil {
			t.Fatalf("switch -c --json: %v", err)
		}
	})

	var env JSONEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if env.Command != "switch" || env.Status != "ok" {
		t.Errorf("envelope = %+v, want command=switch status=ok", env)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}
	if data["branch"] != "dev" {
		t.Errorf("data.branch = %v, want 'dev'", data["branch"])
	}
}
