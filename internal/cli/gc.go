package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewGCCmd creates the gc subcommand.
func NewGCCmd(application *apppkg.App) *cobra.Command {
	var (
		dryRun    bool
		verbose   bool
	)

	cmd := &cobra.Command{
		Use:   "gc",
		Short: "Clean up unreachable objects and optimize the repository",
		Long: `Remove objects that are no longer reachable from any branch, tag,
or operation log entry. This reclaims disk space used by amended
commits, deleted branches, and other orphaned data.

Objects referenced by the operation log (reflog) are preserved so
that 'drift undo' continues to work.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !application.IsInitialized() {
				return fmt.Errorf("not a drift project (run 'drift init')")
			}

			result, err := application.GC(apppkg.GCOptions{
				DryRun:  dryRun,
				Verbose: verbose || dryRun,
			})
			if err != nil {
				return err
			}

			if dryRun {
				fmt.Printf("Would remove %d object(s) (%d bytes)\n", result.ObjectsRemoved, result.BytesFreed)
			} else {
				if result.ObjectsRemoved == 0 {
					fmt.Println("Nothing to clean up.")
				} else {
					fmt.Printf("Removed %d object(s) (%d bytes freed)\n", result.ObjectsRemoved, result.BytesFreed)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report what would be deleted without actually removing")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Print per-object details")

	return cmd
}
