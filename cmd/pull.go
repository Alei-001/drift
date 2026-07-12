package cmd

import (
	"fmt"
	"strings"

	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/spf13/cobra"
)

var pullAll bool
var pullDryRun bool
var pullRestore bool

var pullCmd = &cobra.Command{
	Use:   "pull <remote> [--branch <name>]",
	Short: "Download snapshots, chunks, and refs from a remote",
	Long: `Download remote repository objects to the local store.

By default only the current branch is pulled. Use --all to pull all branches.
Objects already present locally are skipped. Diverged refs (same name,
different target) are saved as <name>.remote so you can inspect them.

HEAD and config are NOT synced. With --restore, working directory files are
automatically updated to the latest branch tip after pulling (equivalent to
running 'drift restore head'). Without --restore, use 'drift restore head'.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		remoteName := args[0]
		branch, _ := cmd.Flags().GetString("branch")

		ctx := cmd.Context()
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		store, cfg, err := openProjectOrReport("Pull", "pull", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		if pullDryRun {
			stats, err := porcelain.PullDryRun(ctx, store, cwd, remoteName, branch, pullAll)
			if err != nil {
				statusFailed("Pull", err.Error(), "check remote configuration and network connectivity")
				return ErrSilent
			}
			statusOK("Pull (dry run)")
			fmt.Printf("  snapshots:  %d would download, %d already present\n", stats.SnapshotsUploaded, stats.SnapshotsSkipped)
			fmt.Printf("  chunks:     %d would download, %d already present\n", stats.ChunksUploaded, stats.ChunksSkipped)
			fmt.Printf("  refs:       %d would update, %d diverged\n", stats.RefsUpdated, stats.RefsDiverged)
			return nil
		}

		result, err := porcelain.PullFromRemote(ctx, store, cwd, remoteName, branch, pullAll)
		if err != nil {
			statusFailed("Pull", err.Error(), "check remote configuration and network connectivity")
			return ErrSilent
		}
		stats := result.Stats
		statusOK("Pulling from '%s'", remoteName)
		fmt.Printf("  snapshots:  %d downloaded, %d already present\n", stats.SnapshotsUploaded, stats.SnapshotsSkipped)
		fmt.Printf("  chunks:     %d downloaded, %d already present\n", stats.ChunksUploaded, stats.ChunksSkipped)
		fmt.Printf("  refs:       %d updated, %d diverged (saved as .remote)\n", stats.RefsUpdated, stats.RefsDiverged)
		if stats.IndexRebuilt {
			displayName := strings.TrimPrefix(stats.BranchTipChanged, "heads/")
			fmt.Printf("  index:      rebuilt (branch '%s' tip advanced)\n", displayName)
		}

		if pullRestore && stats.IndexRebuilt {
			snapshot := porcelain.ResolveHeadSnapshot(ctx, store)
			if snapshot != nil {
				if _, err := porcelain.RestoreSnapshot(ctx, store, cwd, snapshot.ID, "", false, &cfg.Core); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: restore failed: %v (run 'drift restore head' manually)\n", err)
				} else {
					statusOK("Working directory restored")
				}
			}
		} else if stats.IndexRebuilt {
			displayName := strings.TrimPrefix(stats.BranchTipChanged, "heads/")
			fmt.Printf("  hint: branch '%s' tip advanced. Working directory is out of sync.\n", displayName)
			fmt.Printf("        run 'drift restore head' to update your files.\n")
		}
		return nil
	},
}

func init() {
	pullCmd.Flags().StringP("branch", "b", "", "pull only the specified branch (default: current branch)")
	pullCmd.Flags().BoolVar(&pullAll, "all", false, "pull all branches")
	pullCmd.Flags().BoolVar(&pullDryRun, "dry-run", false, "show what would be pulled without downloading")
	pullCmd.Flags().BoolVar(&pullRestore, "restore", false, "automatically restore working directory files after pull")
	rootCmd.AddCommand(pullCmd)
}
