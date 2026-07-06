package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/internal/porcelain"
)

var diffCmd = &cobra.Command{
	Use:   "diff [<id1>] [<id2>] [<file>]",
	Short: "Show changes between snapshots or workspace",
	Args:  cobra.MaximumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		store, cfg, err := openProjectOrReport("Diff", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		if len(args) == 0 {
			snap := porcelain.ResolveHeadSnapshot(ctx, store)
			if snap == nil {
				statusFailed("Diff", "no snapshot to compare against.", "use 'drift save -m \"message\"' to create one first.")
				return ErrSilent
			}
			return porcelain.DiffWorkspaceVsSnapshot(store, cwd, snap, &cfg.Core)
		} else if len(args) == 1 {
			// Try as snapshot ID first
			snap1 := resolveSnapshot(ctx, store, args[0])
			if snap1 != nil {
				return porcelain.DiffWorkspaceVsSnapshot(store, cwd, snap1, &cfg.Core)
			}
			// Fall back: treat as file path, compare with HEAD
			headSnap := porcelain.ResolveHeadSnapshot(ctx, store)
			if headSnap == nil {
				statusFailed("Diff", "no snapshot to compare against.", "use 'drift save -m \"message\"' to create one first.")
				return ErrSilent
			}
			return porcelain.DiffWorkspaceFileVsSnapshot(ctx, store, cwd, headSnap, args[0])
		} else if len(args) == 3 {
			snap1 := resolveSnapshot(ctx, store, args[0])
			if snap1 == nil {
				statusFailed("Diff", fmt.Sprintf("snapshot '%s' not found.", args[0]), "use 'drift log' to list available snapshots.")
				return ErrSilent
			}
			snap2 := resolveSnapshot(ctx, store, args[1])
			if snap2 == nil {
				statusFailed("Diff", fmt.Sprintf("snapshot '%s' not found.", args[1]), "use 'drift log' to list available snapshots.")
				return ErrSilent
			}
			filePath := args[2]
			porcelain.DiffFileInSnapshots(ctx, store, cwd, snap1, snap2, filePath)
		} else {
			snap1 := resolveSnapshot(ctx, store, args[0])
			if snap1 == nil {
				statusFailed("Diff", fmt.Sprintf("snapshot '%s' not found.", args[0]), "use 'drift log' to list available snapshots.")
				return ErrSilent
			}
			snap2 := resolveSnapshot(ctx, store, args[1])
			if snap2 == nil {
				statusFailed("Diff", fmt.Sprintf("snapshot '%s' not found.", args[1]), "use 'drift log' to list available snapshots.")
				return ErrSilent
			}
			porcelain.DiffSnapshots(store, snap1, snap2)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(diffCmd)
}
