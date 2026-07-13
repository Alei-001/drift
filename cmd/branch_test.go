package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestBranch_List(t *testing.T) {
	setupTestRepo(t)

	out := captureStdout(t, func() {
		if err := runCmd(branchListCmd, nil); err != nil {
			t.Fatalf("branch list: %v", err)
		}
	})

	if !strings.Contains(out, "main") {
		t.Errorf("stdout = %q, want 'main'", out)
	}
	if !strings.Contains(out, "*") {
		t.Errorf("stdout = %q, want '*' marking current branch", out)
	}
}

func TestBranch_Create(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "f.txt", "data", "initial")

	out := captureStdout(t, func() {
		if err := runCmd(branchCreateCmd, []string{"feature"}); err != nil {
			t.Fatalf("branch create: %v", err)
		}
	})

	if !strings.Contains(out, "Branch created") {
		t.Errorf("stdout = %q, want 'Branch created'", out)
	}
	if !strings.Contains(out, "feature") {
		t.Errorf("stdout = %q, want 'feature'", out)
	}

	// Verify the branch shows up in the list.
	listOut := captureStdout(t, func() {
		if err := runCmd(branchListCmd, nil); err != nil {
			t.Fatalf("branch list: %v", err)
		}
	})
	if !strings.Contains(listOut, "feature") {
		t.Errorf("branch list = %q, want 'feature'", listOut)
	}
}

func TestBranch_CreateExisting(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "f.txt", "data", "initial")

	errOut := captureStderr(t, func() {
		if err := runCmd(branchCreateCmd, []string{"main"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "already exists") {
		t.Errorf("stderr = %q, want 'already exists'", strings.TrimSpace(errOut))
	}
}

func TestBranch_Delete(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "f.txt", "data", "initial")

	// Create a branch to delete.
	_ = captureStdout(t, func() {
		if err := runCmd(branchCreateCmd, []string{"temp"}); err != nil {
			t.Fatalf("branch create: %v", err)
		}
	})

	out := captureStdout(t, func() {
		if err := runCmd(branchDeleteCmd, []string{"temp"}); err != nil {
			t.Fatalf("branch delete: %v", err)
		}
	})

	if !strings.Contains(out, "Branch deleted") {
		t.Errorf("stdout = %q, want 'Branch deleted'", out)
	}
}

func TestBranch_DeleteMain(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "f.txt", "data", "initial")

	errOut := captureStderr(t, func() {
		if err := runCmd(branchDeleteCmd, []string{"main"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "cannot delete") {
		t.Errorf("stderr = %q, want 'cannot delete'", strings.TrimSpace(errOut))
	}
}

func TestBranch_JSON(t *testing.T) {
	setupTestRepo(t)
	globalJSON = true

	out := captureStdout(t, func() {
		if err := runCmd(branchListCmd, nil); err != nil {
			t.Fatalf("branch list: %v", err)
		}
	})

	var env JSONEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if env.Command != "branch" || env.Status != "ok" {
		t.Errorf("envelope = %+v, want command=branch status=ok", env)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}
	branches, _ := data["branches"].([]interface{})
	if len(branches) != 1 {
		t.Errorf("data.branches has %d entries, want 1", len(branches))
	}
}
