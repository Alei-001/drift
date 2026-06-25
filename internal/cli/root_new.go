package cli

import (
	"fmt"
	"os"

	"github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// BuildRootCmd creates the root cobra command wired to the App layer.
// Old package-level rootCmd and init() registrations remain intact for
// test compatibility but are no longer invoked by main.go.
func BuildRootCmd(application *app.App) *cobra.Command {
	var (
		globalRepoPath string
		globalVerbose  bool
		globalQuiet    bool
		globalDryRun   bool
		globalNoColor  bool
	)

	root := &cobra.Command{
		Use:   "drift",
		Short: "Drift - A lightweight version control tool for creative workers",
		Long:  "Drift lets creative workers manage their work like developers manage code.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Bare "drift" with no subcommand: initialize if needed, else help.
			if !application.IsInitialized() {
				return application.Init()
			}
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Handle -C/--directory: change process cwd first so all
			// downstream os.Getwd()/relative-path logic sees the target,
			// then update App's internal state to match.
			if globalRepoPath != "" {
				if err := os.Chdir(globalRepoPath); err != nil {
					return fmt.Errorf("cannot change to %q: %w", globalRepoPath, err)
				}
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				if err := application.Chdir(cwd); err != nil {
					return err
				}
			}

			// --help on any command: skip init check so help works
			// even outside a repository.
			if helpFlag, _ := cmd.Flags().GetBool("help"); helpFlag {
				return nil
			}

			// MAINTENANCE: when adding a new command that works without
			// an initialized repository, add its case here.
			// See docs/refactoring/03-phase3-cli-framework.md.
			switch cmd.Name() {
			case "drift", "init", "help", "version", "clone":
				return nil
			case "config":
				if global, _ := cmd.Flags().GetBool("global"); global {
					return nil
				}
			case "remote":
				// "drift sync remote" — parent is "sync", no repo needed.
				if cmd.Parent() != nil && cmd.Parent().Name() == "sync" {
					return nil
				}
			}

			if !application.IsInitialized() {
				return fmt.Errorf("not a drift repository (run 'drift init')")
			}
			return nil
		},
	}

	// Persistent flags — names match the old CLI for user-facing compatibility.
	root.PersistentFlags().StringVarP(&globalRepoPath, "directory", "C", "", "Run as if drift was started in <path>")
	root.PersistentFlags().BoolVarP(&globalVerbose, "verbose", "v", false, "Verbose output")
	root.PersistentFlags().BoolVarP(&globalQuiet, "quiet", "q", false, "Quiet output (errors only)")
	root.PersistentFlags().BoolVar(&globalDryRun, "dry-run", false, "Preview without executing")
	root.PersistentFlags().BoolVar(&globalNoColor, "no-color", false, "Disable color output")

	// Register version command (reuses old versionCmd variable from version.go)
	root.AddCommand(versionCmd)

	// Register all new commands
	root.AddCommand(NewInitCmd(application))
	root.AddCommand(NewAddCmd(application))
	root.AddCommand(NewUnstageCmd(application))
	root.AddCommand(NewSaveCmd(application))
	root.AddCommand(NewHistoryCmd(application))
	root.AddCommand(NewUndoCmd(application))
	root.AddCommand(NewLogCmd(application))
	root.AddCommand(NewStatusCmd(application))
	root.AddCommand(NewDiffCmd(application))
	root.AddCommand(NewExportCmd(application))
	root.AddCommand(NewRestoreCmd(application))
	root.AddCommand(NewSwitchCmd(application))
	root.AddCommand(NewBranchCmd(application))
	root.AddCommand(NewNameCmd(application))
	root.AddCommand(NewRmCmd(application))
	root.AddCommand(NewMvCmd(application))
	root.AddCommand(NewWIPCmd(application))
	root.AddCommand(NewCleanCmd(application))
	root.AddCommand(NewCloneCmd(application))
	root.AddCommand(NewConfigCmd(application))
	root.AddCommand(NewSyncCmd(application))

	return root
}