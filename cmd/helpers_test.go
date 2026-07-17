package cmd

import (
	"github.com/Alei-001/drift/internal/project"
	snapkg "github.com/Alei-001/drift/internal/snapshot"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// resetFlags resets all package-level flag variables to their zero/default
// values so tests don't leak flag state into each other. Call this in
// setupTestRepo and at the start of any test that touches flags directly.
func resetFlags() {
	globalCwd = ""
	globalJSON = false
	globalQuiet = false

	saveMessage = ""
	saveTags = nil
	statusShort = false
	logLimit = 0
	logDetail = ""
	logAll = false
	logBranch = ""
	checkVerbose = false
	checkFilter = ""
	gcDryRun = false
	gcKeepAuto = 0
	switchCreate = false
	switchNoAutosave = false
	restoreNoBackup = false
	showOpen = false
	diffStatOnly = false
	exportOutput = ""
	versionVerbose = false
	watchInterval = 0
	watchKeep = 0
	pushAll = false
	pushDryRun = false
	pullAll = false
	pullDryRun = false
	pullRestore = false
}

// setupTestRepo creates a temp dir, initializes a drift repo there, points
// globalCwd at it, and returns the path. The repo is cleaned up automatically.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	workDir := t.TempDir()
	if err := project.InitProject(workDir); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	resetFlags()
	globalCwd = workDir
	t.Cleanup(func() { resetFlags() })
	return workDir
}

// setupEmptyDir creates a temp dir (NOT a drift repo) and points globalCwd at
// it. Used by tests that verify "not a drift repository" error paths.
func setupEmptyDir(t *testing.T) string {
	t.Helper()
	workDir := t.TempDir()
	resetFlags()
	globalCwd = workDir
	t.Cleanup(func() { resetFlags() })
	return workDir
}

// writeFile creates a file under workDir (creating parent directories).
func writeFile(t *testing.T, workDir, relPath, content string) {
	t.Helper()
	full := filepath.Join(workDir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

// saveSnapshot is a fixture helper: it writes a single file and creates a
// snapshot via the save command. Stdout is captured and discarded. The
// short snapshot ID is returned so tests can reference it.
func saveSnapshot(t *testing.T, workDir, filename, content, message string) string {
	t.Helper()
	writeFile(t, workDir, filename, content)
	saveMessage = message
	defer func() { saveMessage = "" }()
	_ = captureStdout(t, func() {
		saveCmd.SetContext(context.Background())
		if err := saveCmd.RunE(saveCmd, nil); err != nil {
			t.Fatalf("saveSnapshot fixture: %v", err)
		}
	})
	// Read back the HEAD snapshot's short ID via porcelain.
	store, _, err := project.OpenProject(workDir)
	if err != nil {
		t.Fatalf("OpenProject: %v", err)
	}
	defer store.Close()
	snap := snapkg.ResolveHeadSnapshot(context.Background(), store)
	if snap == nil {
		t.Fatal("ResolveHeadSnapshot returned nil after save")
	}
	return snap.ShortID()
}

// captureStderr swaps os.Stderr for a pipe, runs fn, and returns whatever was
// written. The original stderr is restored even on panic.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	fn()
	w.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// runCmd is a convenience wrapper that sets a background context on cmd and
// calls its RunE with the given args, returning the error.
func runCmd(cmd *cobra.Command, args []string) error {
	cmd.SetContext(context.Background())
	return cmd.RunE(cmd, args)
}
