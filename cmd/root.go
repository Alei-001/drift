package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Alei-001/drift/internal/version"
)

// Global CLI option flags. These are bound to PersistentFlags in init() and
// are available to all subcommands.
var (
	globalCwd   string
	globalJSON  bool
	globalQuiet bool
)

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
		os.Exit(1)
	}
}
