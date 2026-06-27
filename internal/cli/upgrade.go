package cli

import (
	"fmt"

	"github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewUpgradeCmd creates the "drift upgrade" command.
func NewUpgradeCmd(application *app.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade repository format to the latest version",
		Long: `Upgrade the repository's on-disk format to the latest version
supported by this drift binary.

This is a safe operation: each migration runs under the store lock,
and the version file is checkpointed after every step so interrupted
upgrades can be resumed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !application.IsInitialized() {
				return fmt.Errorf("not a drift repository (run 'drift init')")
			}

			dryRun, _ := cmd.Flags().GetBool("dry-run")

			result, err := application.Upgrade(dryRun)
			if err != nil {
				return err
			}

			if result.AlreadyDone {
				fmt.Printf("Repository is already at format version %d.\n", result.To)
				return nil
			}

			if dryRun {
				fmt.Println("Dry run — no changes made.")
				return nil
			}

			if result.From == 0 {
				fmt.Printf("Repository marked as format version %d (no migration needed).\n", result.To)
				return nil
			}

			fmt.Printf("Upgrade complete: v%d → v%d\n", result.From, result.To)
			return nil
		},
	}

	cmd.Flags().Bool("dry-run", false, "Show what would be done without making changes")
	return cmd
}
