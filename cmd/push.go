package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/Alei-001/drift/internal/porcelain"
)

// pushCmd uploads local objects to a remote.
var pushCmd = &cobra.Command{
	Use:   "push <remote> [--branch <name>]",
	Short: "Upload snapshots, chunks, and refs to a remote",
	Long: `Upload local repository objects to a configured remote.

Objects already present on the remote are skipped. Refs that diverge (same
name, different target) cause an error — pull first to resolve the fork.

HEAD and config are NOT synced: HEAD is per-machine workspace state, and
config is per-repo behavior that each machine may customize independently.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		remoteName := args[0]
		branch, _ := cmd.Flags().GetString("branch")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

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

		if dryRun {
			// Dry-run is not yet implemented in the sync layer; report a
			// clear message rather than silently doing a real push.
			fmt.Fprintln(os.Stderr, "dry-run mode is not yet implemented; omit --dry-run to push for real.")
			return ErrSilent
		}

		result, err := porcelain.PushToRemote(ctx, store, cwd, remoteName, branch)
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
	pushCmd.Flags().StringP("branch", "b", "", "push only the specified branch")
	pushCmd.Flags().Bool("dry-run", false, "show what would be pushed without uploading")
	rootCmd.AddCommand(pushCmd)
}
