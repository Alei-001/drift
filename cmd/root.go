package cmd

import (
	"github.com/Alei-001/drift/internal/project"
	"github.com/Alei-001/drift/internal/errs"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Alei-001/drift/internal/remote"
	"github.com/Alei-001/drift/internal/util/logutil"
	"github.com/Alei-001/drift/internal/version"
)

// Exit codes returned by Execute. They are intentionally stable so that
// scripts and shell pipelines can branch on them. The names mirror the
// failure category, not the specific sentinel, so additional sentinels
// can be added without breaking the contract.
const (
	ExitOK       = 0
	ExitError    = 1 // generic error not covered below
	ExitUsage    = 2 // unknown command/flag or bad arg count
	ExitNotRepo  = 3 // cwd is not a drift repository
	ExitNetwork  = 4 // network error (upgrade, push, pull, clone, ls-remote)
	ExitConflict = 5 // workspace locked, branch/tag already exists, etc.
)

// Global CLI option flags. These are bound to PersistentFlags in init() and
// are available to all subcommands.
var (
	globalCwd     string
	globalJSON    bool
	globalQuiet   bool
	globalVerbose bool
)

// logCloser holds the closer for .drift/logs/drift.log when file logging
// is active. It is closed in Execute() after the command finishes.
var logCloser io.Closer

var rootCmd = &cobra.Command{
	Use:   "drift",
	Short: "Version control for creators",
	Long:  "drift is a version control system designed for creative workflows.",
	// Version populates the built-in --version flag output. We render a
	// short single-line form here; the full version details (commit, build
	// date, platform) are available via `drift version`.
	Version: version.GetInfo().Version,
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Best-effort file logging: if the current directory is a drift
		// project, redirect slog output to .drift/logs/drift.log so
		// operations are recorded for troubleshooting. Commands that
		// don't operate on a project (version, upgrade, init) silently
		// skip logging initialization. Failures are non-fatal: the
		// default stderr logger is used as fallback.
		cwd, err := getCwd()
		if err != nil {
			return nil
		}
		driftDir := filepath.Join(cwd, ".drift")
		if _, err := os.Stat(driftDir); err != nil {
			return nil
		}
		c, err := logutil.InitFileLogger(driftDir)
		if err != nil {
			slog.Warn("init file logger", "error", err)
			return nil
		}
		logCloser = c
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&globalCwd, "cwd", "C", "", "run command in the specified directory")
	rootCmd.PersistentFlags().BoolVar(&globalJSON, "json", false, "output in JSON format")
	rootCmd.PersistentFlags().BoolVarP(&globalQuiet, "quiet", "q", false, "quiet mode (errors only)")
	rootCmd.PersistentFlags().BoolVarP(&globalVerbose, "verbose", "v", false, "show detailed error information")
}

// getCwd returns the working directory for the command. If --cwd is set,
// it returns the absolute path of that directory; otherwise it falls back
// to os.Getwd().
func getCwd() (string, error) {
	if globalCwd != "" {
		abs, err := filepath.Abs(globalCwd)
		if err != nil {
			return "", fmt.Errorf("resolve --cwd: %w", err)
		}
		return abs, nil
	}
	return os.Getwd()
}

// Execute runs the root command and returns a process exit code. The
// caller (main) should pass the return value to os.Exit. Errors are
// classified by sentinel identity so scripts can branch on ExitNotRepo,
// ExitNetwork, ExitConflict, etc. ErrSilent means the error was already
// displayed via reportFailed; it maps to ExitError.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rootCmd.SetContext(ctx)
	err := rootCmd.Execute()
	if logCloser != nil {
		_ = logCloser.Close()
		logCloser = nil
	}
	if err == nil {
		return ExitOK
	}
	return classifyError(err)
}

// classifyError maps a cobra/porcelain error to a stable exit code. The
// mapping is conservative: when in doubt, return ExitError so unexpected
// failures still surface a non-zero status.
func classifyError(err error) int {
	// Check specific error types first — silentWrap makes the error
	// match both ErrSilent and the underlying sentinel, so we must
	// test the more specific types before the generic ErrSilent catch.
	if errors.Is(err, errs.ErrNotARepo) {
		return ExitNotRepo
	}
	if errors.Is(err, version.ErrNetwork) || errors.Is(err, remote.ErrNetwork) {
		return ExitNetwork
	}
	if errors.Is(err, project.ErrLocked) ||
		errors.Is(err, errs.ErrBranchAlreadyExists) ||
		errors.Is(err, errs.ErrTagAlreadyExists) ||
		errors.Is(err, errs.ErrCannotDeleteCurrentBranch) ||
		errors.Is(err, errs.ErrCannotDeleteMain) ||
		errors.Is(err, errs.ErrCannotRenameMain) ||
		errors.Is(err, errs.ErrUncommittedChanges) {
		return ExitConflict
	}
	if isUsageError(err) {
		fmt.Fprintln(os.Stderr, err)
		return ExitUsage
	}
	if errors.Is(err, ErrSilent) {
		return ExitError
	}
	// Default: print and exit 1.
	fmt.Fprintln(os.Stderr, err)
	return ExitError
}

// isUsageError reports whether err is a cobra usage error (unknown
// command, unknown flag, bad argument count). Cobra returns errors
// whose message starts with the standard phrases below; we match them
// here because cobra does not export typed sentinels for these cases.
// This is the documented cmd-layer exception to the no-strings-match
// rule for error classification.
func isUsageError(err error) bool {
	msg := err.Error()
	for _, phrase := range []string{
		"unknown command",
		"unknown flag",
		"unknown shorthand flag",
		"invalid argument",
		"accepts",
		"requires at least",
		"required flag",
	} {
		if containsPhrase(msg, phrase) {
			return true
		}
	}
	return false
}

// containsPhrase reports whether s contains phrase as a substring. It is
// a small helper to keep the import surface minimal.
func containsPhrase(s, phrase string) bool {
	if len(phrase) == 0 || len(s) < len(phrase) {
		return false
	}
	for i := 0; i+len(phrase) <= len(s); i++ {
		if s[i:i+len(phrase)] == phrase {
			return true
		}
	}
	return false
}
