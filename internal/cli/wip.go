package cli

import (
	"fmt"

	"github.com/drift/drift/internal/worktree"
	"github.com/spf13/cobra"
)

// Work-in-progress (WIP) auto-save: when switching branches with pending
// changes, the changes are automatically saved to .drift/wip/<branch>.json
// so the user never loses work. This is a friendly alternative to Git's
// stash — no explicit stash command needed, switch just works.
//
// Storage: .drift/wip/<branch>.json — a serialized index of staged entries.

var wipCmd = &cobra.Command{
	Use:   "wip",
	Short: "List saved work-in-progress across branches",
	Long: `Lists all branches that have saved work-in-progress (WIP).

When you switch branches with pending changes, drift automatically saves them
as WIP. Use this command to see which branches have saved work. Use
'drift restore-wip <branch>' to restore that work.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		branches, err := worktree.ListWIPBranches(sharedStore)
		if err != nil {
			return err
		}

		var found bool
		for _, branch := range branches {
			wip, err := worktree.LoadWIP(sharedStore, branch)
			if err != nil || wip == nil || len(wip.Entries) == 0 {
				continue
			}
			found = true
			fmt.Printf("  %s  %d file(s)\n", branch, len(wip.Entries))
		}

		if !found {
			fmt.Println("No saved work-in-progress")
		}
		return nil
	},
}

var restoreWIPCmd = &cobra.Command{
	Use:   "restore-wip [branch]",
	Short: "Restore work-in-progress saved during a branch switch",
	Long: `Restores the auto-saved work-in-progress for the current (or specified) branch.
When you switch branches with pending changes, drift automatically saves them.
Use this command to restore that work.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		branch := sharedRepo.CurrentBranch()
		if len(args) > 0 {
			branch = args[0]
		}

		wip, err := worktree.LoadWIP(sharedStore, branch)
		if err != nil {
			return err
		}
		if wip == nil || len(wip.Entries) == 0 {
			fmt.Printf("No saved work-in-progress for branch %s\n", branch)
			return nil
		}

		restored, err := sharedRepo.RestoreWIP(branch)
		if err != nil {
			return err
		}
		if restored == 0 {
			fmt.Printf("No saved work-in-progress for branch %s\n", branch)
			return nil
		}

		fmt.Printf("Restored %d file(s) from work-in-progress for %s\n", restored, branch)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(wipCmd)
	rootCmd.AddCommand(restoreWIPCmd)
}
