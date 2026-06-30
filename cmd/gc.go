package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/porcelain"
)

var gcDryRun bool

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Reclaim unreachable snapshots and chunks",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		cwd, _ := os.Getwd()
		store, _, err := porcelain.OpenProject(cwd)
		if err != nil {
			statusFailed("GC", ".drift/ directory not found.", "run 'drift init' first.")
			return nil
		}
		defer store.Close()

		report, err := porcelain.CollectGarbage(ctx, store, cwd, gcDryRun)
		if err != nil {
			statusFailed("GC", err.Error(), "")
			return nil
		}

		if report.SnapshotsRemoved == 0 && report.ChunksRemoved == 0 {
			statusOK("GC")
			fmt.Println("  nothing to reclaim.")
			return nil
		}

		if gcDryRun {
			fmt.Println(">>> GC [dry-run]")
			fmt.Printf("  snapshots:  %d would be removed\n", report.SnapshotsRemoved)
			fmt.Printf("  chunks:     %d would be removed\n", report.ChunksRemoved)
			fmt.Printf("  freed:      ~%s\n", formatSize(report.FreedBytes))
		} else {
			statusOK("GC")
			fmt.Printf("  snapshots:  %d removed\n", report.SnapshotsRemoved)
			fmt.Printf("  chunks:     %d removed\n", report.ChunksRemoved)
			fmt.Printf("  freed:      %s\n", formatSize(report.FreedBytes))
		}
		return nil
	},
}

func init() {
	gcCmd.Flags().BoolVar(&gcDryRun, "dry-run", false, "preview only, do not delete")
	rootCmd.AddCommand(gcCmd)
}
