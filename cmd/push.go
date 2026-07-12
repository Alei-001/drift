package cmd

import (
	"fmt"

	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/spf13/cobra"
)

var pushAll bool
var pushDryRun bool

var pushCmd = &cobra.Command{
	Use:   "push <remote> [--branch <name>]",
	Short: "Upload snapshots, chunks, and refs to a remote",
	Long: `Upload local repository objects to a configured remote.

By default only the current branch is pushed. Use --all to push all branches.
Objects already present on the remote are skipped. Refs that diverge (same
name, different target) cause an error — pull first to resolve the fork.

HEAD and config are NOT synced: HEAD is per-machine workspace state, and
config is per-repo behavior that each machine may customize independently.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		remoteName := args[0]
		branch, _ := cmd.Flags().GetString("branch")

		ctx := cmd.Context()
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}

		store, _, err := openProjectOrReport("Push", "push", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		if pushDryRun {
			stats, err := porcelain.PushDryRun(ctx, store, cwd, remoteName, branch, pushAll)
			if err != nil {
				statusFailed("Push", err.Error(), "check remote configuration and network connectivity")
				return ErrSilent
			}
			statusOK("Push (dry run)")
			fmt.Printf("  snapshots:  %d would upload, %d already present\n", stats.SnapshotsUploaded, stats.SnapshotsSkipped)
			fmt.Printf("  chunks:     %d would upload, %d already present\n", stats.ChunksUploaded, stats.ChunksSkipped)
			fmt.Printf("  refs:       %d would update\n", stats.RefsUpdated)
			return nil
		}

		result, err := porcelain.PushToRemote(ctx, store, cwd, remoteName, branch, pushAll)
		if err != nil {
			statusFailed("Push", err.Error(), "check remote configuration and network connectivity")
			return ErrSilent
		}
		stats := result.Stats
		statusOK("Pushing to '%s'", remoteName)
		fmt.Printf("  snapshots:  %d uploaded, %d already present\n", stats.SnapshotsUploaded, stats.SnapshotsSkipped)
		fmt.Printf("  manifests:  %d uploaded\n", stats.ManifestsUploaded)
		fmt.Printf("  chunks:     %d uploaded, %d already present\n", stats.ChunksUploaded, stats.ChunksSkipped)
		fmt.Printf("  refs:       %d updated\n", stats.RefsUpdated)
		return nil
	},
}

func init() {
	pushCmd.Flags().StringP("branch", "b", "", "push only the specified branch (default: current branch)")
	pushCmd.Flags().BoolVar(&pushAll, "all", false, "push all branches")
	pushCmd.Flags().BoolVar(&pushDryRun, "dry-run", false, "show what would be pushed without uploading")
	rootCmd.AddCommand(pushCmd)
}
