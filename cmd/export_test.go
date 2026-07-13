package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExport_ValidSnapshot(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "file.txt", "export me", "initial")

	// Use a temp file path for the output.
	outPath := filepath.Join(t.TempDir(), "export.zip")
	exportOutput = outPath
	t.Cleanup(func() { exportOutput = "" })

	out := captureStdout(t, func() {
		if err := runCmd(exportCmd, []string{"head"}); err != nil {
			t.Fatalf("export: %v", err)
		}
	})

	if !strings.Contains(out, "Exported") {
		t.Errorf("stdout = %q, want 'Exported'", out)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("expected zip at %s: %v", outPath, err)
	}
}

func TestExport_InvalidSnapshot(t *testing.T) {
	setupTestRepo(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(exportCmd, []string{"id:nonexistent"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not found") {
		t.Errorf("stderr = %q, want 'not found'", strings.TrimSpace(errOut))
	}
}

func TestExport_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(exportCmd, []string{"head"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not a drift repository") {
		t.Errorf("stderr = %q, want 'not a drift repository'", strings.TrimSpace(errOut))
	}
}
