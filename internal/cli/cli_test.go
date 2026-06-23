package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/storage"
)

// TestHelper encapsulates test utilities.
type TestHelper struct {
	T       *testing.T
	Dir     string
	Store   *storage.Store
	Config  *config.Config
	origDir string
}

// NewTestHelper creates a temporary directory and initializes shared state.
func NewTestHelper(t *testing.T) *TestHelper {
	t.Helper()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	store := storage.NewStore(dir)
	cfg := config.DefaultConfig()

	h := &TestHelper{
		T:       t,
		Dir:     dir,
		Store:   store,
		Config:  cfg,
		origDir: origDir,
	}

	t.Cleanup(func() {
		os.Chdir(origDir)
		sharedStore = nil
		sharedConfig = nil
		sharedDir = ""
	})

	return h
}

// InitProject runs `drift init`.
func (h *TestHelper) InitProject() {
	h.T.Helper()
	sharedDir = h.Dir
	sharedStore = h.Store
	sharedConfig = h.Config

	if err := h.Store.Init(); err != nil {
		h.T.Fatalf("init failed: %v", err)
	}
	if err := h.Store.SaveRef("HEAD", "main"); err != nil {
		h.T.Fatalf("failed to set HEAD: %v", err)
	}
}

// SetupSharedState sets the shared CLI variables for command execution.
func (h *TestHelper) SetupSharedState() {
	h.T.Helper()
	sharedDir = h.Dir
	sharedStore = storage.NewStore(h.Dir)
	sharedConfig = h.Config

	// Reset all Cobra flags to default values to avoid state leakage between tests
	// This is critical because Cobra flags are global and persist across tests
	resetAllFlags()
}

// resetAllFlags resets all command flags to their default values
func resetAllFlags() {
	// diff command flags - reset global variables directly
	diffPatch = false
	diffOutput = ""
	diffFilePaths = nil

	// export command flags
	exportCmd.Flags().Set("output", "")
	exportCmd.Flags().Set("format", "dir")

	// restore command flags
	restoreCmd.Flags().Set("force", "false")

	// switch command flags
	switchCmd.Flags().Set("force", "false")
	switchCmd.Flags().Set("create", "false")

	// save command flags - message should be set per call
	saveCmd.Flags().Set("message", "")

	// log command package-level vars
	logOneline = false
	logCount = 0

	// branch command package-level vars
	branchDelete = false
	branchMove = ""
}

// WriteFile creates a file with the given content.
func (h *TestHelper) WriteFile(relPath, content string) {
	h.T.Helper()
	fullPath := filepath.Join(h.Dir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		h.T.Fatalf("failed to create dir for %s: %v", relPath, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		h.T.Fatalf("failed to write file %s: %v", relPath, err)
	}
}

// ReadFile reads a file's content.
func (h *TestHelper) ReadFile(relPath string) string {
	h.T.Helper()
	fullPath := filepath.Join(h.Dir, filepath.FromSlash(relPath))
	data, err := os.ReadFile(fullPath)
	if err != nil {
		h.T.Fatalf("failed to read file %s: %v", relPath, err)
	}
	return string(data)
}

// FileExists checks if a file exists.
func (h *TestHelper) FileExists(relPath string) bool {
	h.T.Helper()
	fullPath := filepath.Join(h.Dir, filepath.FromSlash(relPath))
	_, err := os.Stat(fullPath)
	return err == nil
}

// DirExists checks if a directory exists.
func (h *TestHelper) DirExists(relPath string) bool {
	h.T.Helper()
	fullPath := filepath.Join(h.Dir, filepath.FromSlash(relPath))
	info, err := os.Stat(fullPath)
	return err == nil && info.IsDir()
}

// DeleteFile removes a file.
func (h *TestHelper) DeleteFile(relPath string) {
	h.T.Helper()
	fullPath := filepath.Join(h.Dir, filepath.FromSlash(relPath))
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		h.T.Fatalf("failed to delete file %s: %v", relPath, err)
	}
}

// AssertContains checks if output contains expected string.
func (h *TestHelper) AssertContains(output, expected string) {
	h.T.Helper()
	if !strings.Contains(output, expected) {
		h.T.Errorf("output %q does not contain %q", output, expected)
	}
}

// AssertNotContains checks if output does not contain expected string.
func (h *TestHelper) AssertNotContains(output, expected string) {
	h.T.Helper()
	if strings.Contains(output, expected) {
		h.T.Errorf("output %q should not contain %q", output, expected)
	}
}

// CaptureOutput captures stdout during fn execution.
func CaptureOutput(fn func() error) (string, error) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String(), err
}

// RunInit runs the init command and returns output.
func (h *TestHelper) RunInit() (string, error) {
	h.T.Helper()
	return CaptureOutput(func() error {
		return initCmd.RunE(initCmd, nil)
	})
}

// RunAdd runs the add command with args.
func (h *TestHelper) RunAdd(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		return addCmd.RunE(addCmd, args)
	})
}

// RunStatus runs the status command.
func (h *TestHelper) RunStatus() (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		return statusCmd.RunE(statusCmd, nil)
	})
}

// RunUnstage runs the unstage command.
func (h *TestHelper) RunUnstage() (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		return unstageCmd.RunE(unstageCmd, nil)
	})
}

// RunSave runs the save command with optional -m flag.
func (h *TestHelper) RunSave(message string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	// Always set the message flag (even to empty) to avoid state leakage between tests.
	saveCmd.Flags().Set("message", message)
	return CaptureOutput(func() error {
		return saveCmd.RunE(saveCmd, nil)
	})
}

// RunList runs the list command.
func (h *TestHelper) RunList(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		return listCmd.RunE(listCmd, args)
	})
}

// RunExport runs the export command.
func (h *TestHelper) RunExport(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	// Reset flags to defaults to avoid state leakage between tests.
	exportCmd.Flags().Set("output", "")
	exportCmd.Flags().Set("format", "dir")
	return CaptureOutput(func() error {
		// Parse flags from args
		var filteredArgs []string
		for i := 0; i < len(args); i++ {
			if args[i] == "-o" && i+1 < len(args) {
				exportCmd.Flags().Set("output", args[i+1])
				i++
			} else if args[i] == "-f" && i+1 < len(args) {
				exportCmd.Flags().Set("format", args[i+1])
				i++
			} else {
				filteredArgs = append(filteredArgs, args[i])
			}
		}
		return exportCmd.RunE(exportCmd, filteredArgs)
	})
}

// RunRestore runs the restore command.
func (h *TestHelper) RunRestore(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		// Parse flags from args
		var filteredArgs []string
		for _, arg := range args {
			if arg == "--force" {
				restoreCmd.Flags().Set("force", "true")
			} else {
				filteredArgs = append(filteredArgs, arg)
			}
		}
		return restoreCmd.RunE(restoreCmd, filteredArgs)
	})
}

// RunBranch runs the branch command.
func (h *TestHelper) RunBranch(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	branchDelete = false
	branchMove = ""
	// Parse -d and -m flags from args; they are cobra-via-BoolVarP/StringVarP.
	for i, arg := range args {
		if arg == "-d" || arg == "--delete" {
			branchDelete = true
		}
		if (arg == "-m" || arg == "--move") && i+1 < len(args) {
			branchMove = args[i+1]
		}
	}
	filteredArgs := make([]string, 0, len(args))
	skipNext := false
	for _, arg := range args {
		if arg == "-d" || arg == "--delete" {
			continue
		}
		if arg == "-m" || arg == "--move" {
			skipNext = true
			continue
		}
		if skipNext {
			skipNext = false
			continue
		}
		filteredArgs = append(filteredArgs, arg)
	}
	return CaptureOutput(func() error {
		return branchCmd.RunE(branchCmd, filteredArgs)
	})
}

// RunSwitch runs the switch command.
func (h *TestHelper) RunSwitch(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		// Parse --force and --create/-c from args
		forceFlag := false
		createFlag := false
		for _, arg := range args {
			if arg == "--force" {
				forceFlag = true
			}
			if arg == "--create" || arg == "-c" {
				createFlag = true
			}
		}
		switchCmd.Flags().Set("force", strBool(forceFlag))
		switchCmd.Flags().Set("create", strBool(createFlag))

		// Filter out flags from positional args
		filteredArgs := make([]string, 0, len(args))
		for _, arg := range args {
			if arg == "--force" || arg == "--create" || arg == "-c" {
				continue
			}
			filteredArgs = append(filteredArgs, arg)
		}
		return switchCmd.RunE(switchCmd, filteredArgs)
	})
}

func strBool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// RunDiff runs the diff command.
func (h *TestHelper) RunDiff(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		return diffCmd.RunE(diffCmd, args)
	})
}

// RunDiffWithPatch runs the diff command with --patch flag.
func (h *TestHelper) RunDiffWithPatch(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	diffPatch = true
	return CaptureOutput(func() error {
		return diffCmd.RunE(diffCmd, args)
	})
}

// RunDiffWithFile runs the diff command with --file flag.
func (h *TestHelper) RunDiffWithFile(filePath string, args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	diffFilePaths = []string{filePath}
	return CaptureOutput(func() error {
		return diffCmd.RunE(diffCmd, args)
	})
}

// AddAndSave is a helper to add files and save a version.
func (h *TestHelper) AddAndSave(files []string, message string) string {
	h.T.Helper()
	for _, f := range files {
		if _, err := h.RunAdd(f); err != nil {
			h.T.Fatalf("add %s failed: %v", f, err)
		}
	}
	output, err := h.RunSave(message)
	if err != nil {
		h.T.Fatalf("save failed: %v", err)
	}
	return output
}

// AssertNoError fails the test if err is not nil.
func (h *TestHelper) AssertNoError(err error) {
	h.T.Helper()
	if err != nil {
		h.T.Fatalf("expected no error, got: %v", err)
	}
}

// AssertError fails the test if err is nil.
func (h *TestHelper) AssertError(err error) {
	h.T.Helper()
	if err == nil {
		h.T.Fatal("expected error, got nil")
	}
}

// VersionCount returns the number of commits.
func (h *TestHelper) VersionCount() int {
	h.T.Helper()
	commits, err := h.Store.ListCommits()
	if err != nil {
		h.T.Fatalf("failed to list commits: %v", err)
	}
	return len(commits)
}

// RunLog runs the log command.
func (h *TestHelper) RunLog(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	logOneline = false
	logCount = 0
	return CaptureOutput(func() error {
		return logCmd.RunE(logCmd, args)
	})
}

// RunLogOneline runs the log command with --oneline.
func (h *TestHelper) RunLogOneline(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	logOneline = true
	logCount = 0
	return CaptureOutput(func() error {
		return logCmd.RunE(logCmd, args)
	})
}

// RunLogLimit runs the log command with -n limit.
func (h *TestHelper) RunLogLimit(limit int, args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	logOneline = false
	logCount = limit
	return CaptureOutput(func() error {
		return logCmd.RunE(logCmd, args)
	})
}

// RunConfig runs the config command.
func (h *TestHelper) RunConfig(args ...string) (string, error) {
	h.T.Helper()
	sharedDir = h.Dir
	sharedStore = storage.NewStore(h.Dir)
	sharedConfig = h.Config
	resetAllFlags()
	if !sharedStore.IsInitialized() {
		if err := sharedStore.Init(); err != nil {
			return "", err
		}
		sharedStore.SaveRef("main", "")
		sharedStore.SaveRef("HEAD", "main")
	}
	return CaptureOutput(func() error {
		return configCmd.RunE(configCmd, args)
	})
}

func init() {
	// Suppress cobra output during tests
	initCmd.SetOutput(&bytes.Buffer{})
	addCmd.SetOutput(&bytes.Buffer{})
	statusCmd.SetOutput(&bytes.Buffer{})
	unstageCmd.SetOutput(&bytes.Buffer{})
	saveCmd.SetOutput(&bytes.Buffer{})
	listCmd.SetOutput(&bytes.Buffer{})
	exportCmd.SetOutput(&bytes.Buffer{})
	restoreCmd.SetOutput(&bytes.Buffer{})
	branchCmd.SetOutput(&bytes.Buffer{})
	switchCmd.SetOutput(&bytes.Buffer{})
	diffCmd.SetOutput(&bytes.Buffer{})
	logCmd.SetOutput(&bytes.Buffer{})
	configCmd.SetOutput(&bytes.Buffer{})
}

// Helper to format expected output
func formatExpected(parts ...string) string {
	return strings.Join(parts, "\n")
}

// Suppress unused import warnings
var _ = fmt.Sprintf
var _ = filepath.Join
