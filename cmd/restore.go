package cmd

import (
	"fmt"
	"os"

	"github.com/your-org/drift/porcelain"
	"github.com/your-org/drift/storage/filesystem"
	"github.com/spf13/cobra"
)

var restoreNoBackup bool

var restoreCmd = &cobra.Command{
	Use:   "restore <snapshot-id> [<file>]",
	Short: "Restore files from a snapshot",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("snapshot id required")
		}

		cwd, _ := os.Getwd()
		store, _, err := porcelain.OpenProject(cwd)
		if err != nil {
			return err
		}
		defer store.(*filesystem.FSStorage).Close()

		snapshot := resolveSnapshot(store, args[0])
		if snapshot == nil {
			return fmt.Errorf("snapshot not found: %s", args[0])
		}

		var filePath string
		if len(args) > 1 {
			filePath = args[1]
		}

		err = porcelain.RestoreSnapshot(store, cwd, snapshot.ID, filePath, restoreNoBackup)
		if err != nil {
			return err
		}

		if filePath != "" {
			fmt.Printf("Restored %s from snapshot %s\n", filePath, snapshot.ShortID())
		} else {
			fmt.Printf("Restored to snapshot %s: %s\n", snapshot.ShortID(), snapshot.Message)
		}
		return nil
	},
}

func init() {
	restoreCmd.Flags().BoolVar(&restoreNoBackup, "no-backup", false, "skip automatic backup before restore")
	rootCmd.AddCommand(restoreCmd)
}
