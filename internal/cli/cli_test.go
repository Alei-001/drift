package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/repo"
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
		sharedRepo = nil
	})

	return h
}

// InitProject runs `drift init`.
func (h *TestHelper) InitProject() {
	h.T.Helper()
	sharedDir = h.Dir
	sharedStore = h.Store
	sharedConfig = h.Config
	sharedRepo = repo.New(h.Store, h.Config, h.Dir)

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
	sharedRepo = repo.New(sharedStore, h.Config, h.Dir)

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
	saveCmd.Flags().Set("name", "")
	saveCmd.Flags().Set("all", "false")
	saveCmd.Flags().Set("amend", "false")

	// status command package-level vars
	statusPorcelain = false

	// history command package-level vars (commit history)
	historyOneline = false
	historyAll = false
	historyCount = 0
	historyPorcelain = false

	// log command package-level vars (operation log)
	logLimit = 20
	logPorcelain = false

	// branch command package-level vars
	branchDelete = false
	branchMove = ""

	// rm command package-level vars
	rmCached = false
	rmRecursive = false
	rmForce = false

	// mv command package-level vars
	mvForce = false

	// clean command package-level vars
	cleanDirs = false
	cleanForce = false
	cleanDryRun = false

	// config command package-level vars
	configList = false
	configUnset = false
	configGlobal = false

	// name command flags
	nameCmd.Flags().Set("list", "false")
	nameCmd.Flags().Set("delete", "")

	// sync command flags
	syncShowRemote = false
	syncUnsetRemote = false
	syncProtocol = ""
	syncHost = ""
	syncPort = 0
	syncPath = ""
	syncUser = ""
	syncPass = ""
	syncTLS = false
	syncInsecure = false
	syncShare = ""
	syncKeyPath = ""

	// undo command flags
	undoCount = 1

	// wip drop command flag
	wipDropForce = false

	// global persistent flags
	globalVerbose = false
	globalQuiet = false
	globalDryRun = false
	globalNoColor = false
	globalRepoPath = ""
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
// It also redirects stdin to an empty pipe so that isInteractive() returns
// false and confirmAction auto-proceeds — matching non-interactive test mode.
func CaptureOutput(fn func() error) (string, error) {
	oldOut := os.Stdout
	oldIn := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdout = w
	// Redirect stdin to an empty pipe so confirmAction sees a non-TTY
	// and auto-proceeds without blocking on user input.
	inR, inW, _ := os.Pipe()
	inW.Close()
	os.Stdin = inR

	err := fn()

	w.Close()
	os.Stdout = oldOut
	os.Stdin = oldIn

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

// RunUnstage runs the unstage command with optional path args.
func (h *TestHelper) RunUnstage(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		return unstageCmd.RunE(unstageCmd, args)
	})
}

// RunSave runs the save command with optional -m flag.
func (h *TestHelper) RunSave(message string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	// Always set the message flag (even to empty) to avoid state leakage between tests.
	saveCmd.Flags().Set("message", message)
	saveCmd.Flags().Set("name", "")
	saveCmd.Flags().Set("all", "false")
	return CaptureOutput(func() error {
		return saveCmd.RunE(saveCmd, nil)
	})
}

// RunSaveWithName saves with a message and assigns a version name.
func (h *TestHelper) RunSaveWithName(message, name string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	saveCmd.Flags().Set("message", message)
	saveCmd.Flags().Set("name", name)
	saveCmd.Flags().Set("all", "false")
	return CaptureOutput(func() error {
		return saveCmd.RunE(saveCmd, nil)
	})
}

// RunSaveAll runs the save command with --all (auto-stage before saving).
func (h *TestHelper) RunSaveAll(message string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	saveCmd.Flags().Set("message", message)
	saveCmd.Flags().Set("name", "")
	saveCmd.Flags().Set("all", "true")
	return CaptureOutput(func() error {
		return saveCmd.RunE(saveCmd, nil)
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

// RunName runs the name command.
func (h *TestHelper) RunName(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	nameCmd.Flags().Set("list", "false")
	nameCmd.Flags().Set("delete", "")
	var filteredArgs []string
	for _, arg := range args {
		switch {
		case arg == "--list":
			nameCmd.Flags().Set("list", "true")
		case strings.HasPrefix(arg, "--delete="):
			nameCmd.Flags().Set("delete", strings.TrimPrefix(arg, "--delete="))
		case arg == "--delete":
			// Skip; handled by next arg
		default:
			filteredArgs = append(filteredArgs, arg)
		}
	}
	return CaptureOutput(func() error {
		return nameCmd.RunE(nameCmd, filteredArgs)
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
	return h.ExtractSaveID(output)
}

// ExtractSaveID parses the commit ID from save command output.
// The output format is "Saved version <ID>: <message>" or "Saved version <ID>".
func (h *TestHelper) ExtractSaveID(output string) string {
	h.T.Helper()
	const prefix = "Saved version "
	idx := strings.Index(output, prefix)
	if idx < 0 {
		h.T.Fatalf("could not find save ID in output: %s", output)
	}
	rest := output[idx+len(prefix):]
	end := len(rest)
	for i, c := range rest {
		if c == ':' || c == '\n' || c == ' ' {
			end = i
			break
		}
	}
	return rest[:end]
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

// RunHistory runs the history command (commit history).
func (h *TestHelper) RunHistory(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	historyOneline = false
	historyAll = false
	historyCount = 0
	historyPorcelain = false
	return CaptureOutput(func() error {
		return historyCmd.RunE(historyCmd, args)
	})
}

// RunHistoryOneline runs the history command with --oneline.
func (h *TestHelper) RunHistoryOneline(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	historyOneline = true
	historyAll = false
	historyCount = 0
	historyPorcelain = false
	return CaptureOutput(func() error {
		return historyCmd.RunE(historyCmd, args)
	})
}

// RunHistoryAll runs the history command with --all (replaces former RunList).
func (h *TestHelper) RunHistoryAll(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	historyOneline = false
	historyAll = true
	historyCount = 0
	historyPorcelain = false
	return CaptureOutput(func() error {
		return historyCmd.RunE(historyCmd, args)
	})
}

// RunHistoryLimit runs the history command with -n limit.
func (h *TestHelper) RunHistoryLimit(limit int, args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	historyOneline = false
	historyAll = false
	historyCount = limit
	historyPorcelain = false
	return CaptureOutput(func() error {
		return historyCmd.RunE(historyCmd, args)
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
	// Parse --list and --unset flags from args via cobra's flag set, so
	// the bound variables are updated consistently.
	var filteredArgs []string
	for _, arg := range args {
		switch arg {
		case "--list":
			configCmd.Flags().Set("list", "true")
		case "--unset":
			configCmd.Flags().Set("unset", "true")
		default:
			filteredArgs = append(filteredArgs, arg)
		}
	}
	return CaptureOutput(func() error {
		return configCmd.RunE(configCmd, filteredArgs)
	})
}

// RunRm runs the rm command with args.
func (h *TestHelper) RunRm(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		// Parse --cached, -r/--recursive, and -f/--force flags from args.
		var filteredArgs []string
		for _, arg := range args {
			switch arg {
			case "--cached":
				rmCached = true
			case "-r", "--recursive":
				rmRecursive = true
			case "-f", "--force":
				rmForce = true
			default:
				filteredArgs = append(filteredArgs, arg)
			}
		}
		return rmCmd.RunE(rmCmd, filteredArgs)
	})
}

// RunMv runs the mv command with args.
func (h *TestHelper) RunMv(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		// Parse -f/--force flag from args.
		var filteredArgs []string
		for _, arg := range args {
			if arg == "-f" || arg == "--force" {
				mvForce = true
			} else {
				filteredArgs = append(filteredArgs, arg)
			}
		}
		return mvCmd.RunE(mvCmd, filteredArgs)
	})
}

// RunClean runs the clean command with args.
func (h *TestHelper) RunClean(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	cleanDirs = false
	cleanForce = false
	cleanDryRun = false
	for _, arg := range args {
		switch arg {
		case "-d", "--dirs":
			cleanDirs = true
		case "-f", "--force":
			cleanForce = true
		case "-n", "--dry-run":
			cleanDryRun = true
		}
	}
	return CaptureOutput(func() error {
		return cleanCmd.RunE(cleanCmd, nil)
	})
}

// RunLog runs the log command (operation log) with the given limit (0 = all).
func (h *TestHelper) RunLog(limit int) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	logLimit = limit
	logPorcelain = false
	return CaptureOutput(func() error {
		return logCmd.RunE(logCmd, nil)
	})
}

// RunUndo runs the undo command with the given count.
func (h *TestHelper) RunUndo(count int) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	undoCount = count
	return CaptureOutput(func() error {
		return undoCmd.RunE(undoCmd, nil)
	})
}

// RunWipList runs the 'wip list' subcommand.
func (h *TestHelper) RunWipList() (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		return wipListCmd.RunE(wipListCmd, nil)
	})
}

// RunWipSave runs the 'wip save' subcommand.
func (h *TestHelper) RunWipSave() (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		return wipSaveCmd.RunE(wipSaveCmd, nil)
	})
}

// RunWipRestore runs the 'wip restore' subcommand with an optional branch.
func (h *TestHelper) RunWipRestore(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		return wipRestoreCmd.RunE(wipRestoreCmd, args)
	})
}

// RunWipDrop runs the 'wip drop' subcommand with an optional branch.
// Pass "--force" in args to skip the confirmation prompt.
func (h *TestHelper) RunWipDrop(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	var filteredArgs []string
	for _, arg := range args {
		if arg == "--force" || arg == "-f" {
			wipDropCmd.Flags().Set("force", "true")
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}
	return CaptureOutput(func() error {
		return wipDropCmd.RunE(wipDropCmd, filteredArgs)
	})
}

func init() {
	// Suppress cobra output during tests
	initCmd.SetOut(io.Discard)
	initCmd.SetErr(io.Discard)
	addCmd.SetOut(io.Discard)
	addCmd.SetErr(io.Discard)
	statusCmd.SetOut(io.Discard)
	statusCmd.SetErr(io.Discard)
	unstageCmd.SetOut(io.Discard)
	unstageCmd.SetErr(io.Discard)
	saveCmd.SetOut(io.Discard)
	saveCmd.SetErr(io.Discard)
	exportCmd.SetOut(io.Discard)
	exportCmd.SetErr(io.Discard)
	restoreCmd.SetOut(io.Discard)
	restoreCmd.SetErr(io.Discard)
	branchCmd.SetOut(io.Discard)
	branchCmd.SetErr(io.Discard)
	switchCmd.SetOut(io.Discard)
	switchCmd.SetErr(io.Discard)
	diffCmd.SetOut(io.Discard)
	diffCmd.SetErr(io.Discard)
	logCmd.SetOut(io.Discard)
	logCmd.SetErr(io.Discard)
	configCmd.SetOut(io.Discard)
	configCmd.SetErr(io.Discard)
	rmCmd.SetOut(io.Discard)
	rmCmd.SetErr(io.Discard)
	mvCmd.SetOut(io.Discard)
	mvCmd.SetErr(io.Discard)
	cleanCmd.SetOut(io.Discard)
	cleanCmd.SetErr(io.Discard)
	wipListCmd.SetOut(io.Discard)
	wipListCmd.SetErr(io.Discard)
	wipSaveCmd.SetOut(io.Discard)
	wipSaveCmd.SetErr(io.Discard)
	wipRestoreCmd.SetOut(io.Discard)
	wipRestoreCmd.SetErr(io.Discard)
	wipDropCmd.SetOut(io.Discard)
	wipDropCmd.SetErr(io.Discard)
}

// Helper to format expected output
func formatExpected(parts ...string) string {
	return strings.Join(parts, "\n")
}
