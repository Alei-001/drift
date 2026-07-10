package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Alei-001/drift/internal/util/logutil"
	"github.com/Alei-001/drift/internal/version"
)

// Global CLI option flags. These are bound to PersistentFlags in init() and
// are available to all subcommands.
var (
	globalCwd   string
	globalJSON  bool
	globalQuiet bool
)

// logFile holds the file handle for .drift/logs/drift.log when file logging
// is active. It is closed in Execute() after the command finishes.
var logFile *os.File

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
		cwd, err := getCwd(cmd)
		if err != nil {
			return nil
		}
		driftDir := filepath.Join(cwd, ".drift")
		if _, err := os.Stat(driftDir); err != nil {
			return nil
		}
		f, err := logutil.InitFileLogger(driftDir)
		if err != nil {
			slog.Warn("init file logger", "error", err)
			return nil
		}
		logFile = f
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
}

// getCwd returns the working directory for the command. If --cwd is set,
// it returns the absolute path of that directory; otherwise it falls back
// to os.Getwd(). The cmd parameter is accepted for future per-command
// overrides and is currently unused.
func getCwd(cmd *cobra.Command) (string, error) {
	_ = cmd
	if globalCwd != "" {
		abs, err := filepath.Abs(globalCwd)
		if err != nil {
			return "", fmt.Errorf("resolve --cwd: %w", err)
		}
		return abs, nil
	}
	return os.Getwd()
}

// Execute runs the root command and handles exit codes.
func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rootCmd.SetContext(ctx)
	if err := rootCmd.Execute(); err != nil {
		if !errors.Is(err, ErrSilent) {
			fmt.Fprintln(os.Stderr, err)
		}
		if logFile != nil {
			logFile.Close()
		}
		os.Exit(1)
	}
	if logFile != nil {
		logFile.Close()
	}
}
