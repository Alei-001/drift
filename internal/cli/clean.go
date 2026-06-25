package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

var (
	cleanDirs   bool
	cleanForce  bool
	cleanDryRun bool
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove untracked files from the working tree",
	Long: `Remove untracked files from the working tree.

This command deletes files that are not tracked (not staged and not committed).
Use -n for a dry run to preview which files would be deleted.

Examples:
  drift clean          # prompt before deleting untracked files
  drift clean -n       # dry run, list files without deleting
  drift clean -f       # skip confirmation prompt
  drift clean -d       # also remove empty directories`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// First, do a dry run to get the list of files that would be deleted.
		files, err := sharedRepo.WT.CleanUntracked(cleanDirs, true)
		if err != nil {
			return err
		}
		sort.Strings(files)

		if len(files) == 0 {
			fmt.Println("No untracked files to clean")
			return nil
		}

		if cleanDryRun {
			fmt.Printf("Would delete %d untracked file(s):\n", len(files))
			for _, f := range files {
				fmt.Printf("  %s\n", f)
			}
			return nil
		}

		// Confirm before deleting.
		if !confirmAction(cleanForce, fmt.Sprintf("Delete %d untracked file(s)?", len(files)), files) {
			fmt.Println("Aborted")
			return nil
		}

		// Actually delete.
		deleted, err := sharedRepo.WT.CleanUntracked(cleanDirs, false)
		if err != nil {
			return err
		}
		sort.Strings(deleted)

		for _, f := range deleted {
			fmt.Printf("Deleted: %s\n", f)
		}
		fmt.Printf("Deleted %d untracked file(s)\n", len(deleted))
		return nil
	},
}

func init() {
	cleanCmd.Flags().BoolVarP(&cleanDirs, "dirs", "d", false, "Also remove empty directories")
	cleanCmd.Flags().BoolVarP(&cleanForce, "force", "f", false, "Skip confirmation prompt")
	cleanCmd.Flags().BoolVarP(&cleanDryRun, "dry-run", "n", false, "Preview without deleting")
	rootCmd.AddCommand(cleanCmd)
}
