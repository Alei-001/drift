package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/your-org/drift/porcelain"
	"github.com/spf13/cobra"
)

var restoreNoBackup bool

var restoreCmd = &cobra.Command{
	Use:   "restore <snapshot-id> [<file>]",
	Short: "Restore files from a snapshot",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		if len(args) < 1 {
			return fmt.Errorf("snapshot id required")
		}

		cwd, _ := os.Getwd()
		store, _, err := porcelain.OpenProject(cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		snapshot := resolveSnapshot(ctx, store, args[0])
		if snapshot == nil {
			statusFailed("Restore", fmt.Sprintf("snapshot '%s' not found.", args[0]), "use 'drift log' to list available snapshots.")
			return fmt.Errorf("snapshot not found: %s", args[0])
		}

		var filePath string
		if len(args) > 1 {
			filePath = args[1]
		}

		backupID, err := porcelain.RestoreSnapshot(store, cwd, snapshot.ID, filePath, restoreNoBackup)
		if err != nil {
			statusFailed("Restore", "uncommitted changes would be overwritten.", "use 'drift save' first, or restore a single file.")
			return err
		}

		if filePath != "" {
			fmt.Printf(">>> Restored %s:%s [ok]\n", snapshot.ShortID(), filePath)
			fmt.Println()
			fmt.Printf("  ~  %s\n", filePath)
			fmt.Println()
			fmt.Println("  1 file: ~1")
			if backupID != "" {
				fmt.Printf("  backup: [%s]\n", backupID)
			}
		} else {
			// Full restore: show added/modified/deleted files
			add, mod, del := computeChanges(ctx, store, snapshot)
			fmt.Printf(">>> Restored to %s [ok]\n", snapshot.ShortID())
			printFileListWithSize(add, mod, del)
			total := len(add) + len(mod) + len(del)
			summaryLine(total, len(add), len(mod), len(del))
			if backupID != "" {
				fmt.Printf("  backup: [%s]\n", backupID)
			}
		}
		return nil
	},
}

func init() {
	restoreCmd.Flags().BoolVar(&restoreNoBackup, "no-backup", false, "skip automatic backup before restore")
	rootCmd.AddCommand(restoreCmd)
}
