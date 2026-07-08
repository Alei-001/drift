package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/Alei-001/drift/internal/porcelain"
)

// pullCmd downloads remote objects to local.
var pullCmd = &cobra.Command{
	Use:   "pull <remote> [--branch <name>]",
	Short: "Download snapshots, chunks, and refs from a remote",
	Long: `Download remote repository objects to the local store.

Objects already present locally are skipped. Diverged refs (same name,
different target) are saved as <name>.remote so you can inspect them. After
pulling, if the current branch tip advanced, the local index is rebuilt.

HEAD and config are NOT synced. Pull does NOT modify your working directory
files — if the branch tip advanced, run 'drift restore' to update them.`,
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
		store, _, err := openProjectOrReport("Pull", "pull", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		if dryRun {
			fmt.Fprintln(os.Stderr, "dry-run mode is not yet implemented; omit --dry-run to pull for real.")
			return ErrSilent
		}

		result, err := porcelain.PullFromRemote(ctx, store, cwd, remoteName, branch)
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
			// Strip "heads/" prefix for display; BranchTipChanged stores the full ref name.
			displayName := strings.TrimPrefix(stats.BranchTipChanged, "heads/")
			fmt.Printf("  index:      rebuilt (branch '%s' tip advanced)\n", displayName)
			fmt.Printf("  hint: branch '%s' tip advanced. Working directory is out of sync.\n", displayName)
			fmt.Printf("        run 'drift restore head' to update your files.\n")
		}
		return nil
	},
}

func init() {
	pullCmd.Flags().StringP("branch", "b", "", "pull only the specified branch")
	pullCmd.Flags().Bool("dry-run", false, "show what would be pulled without downloading")
	rootCmd.AddCommand(pullCmd)
}
