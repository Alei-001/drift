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

// wipDropForce tracks the --force/-f flag for the 'wip drop' subcommand.
var wipDropForce bool

var wipCmd = &cobra.Command{
	Use:   "wip",
	Short: "Manage saved work-in-progress",
	Long: `Manage work-in-progress (WIP) saved across branches.

When you switch branches with pending changes, drift automatically saves them
as WIP. Use the subcommands below to list, save, restore, or drop WIP.`,
	Args: cobra.NoArgs,
}

var wipListCmd = &cobra.Command{
	Use:   "list",
	Short: "List branches with saved work-in-progress",
	Long: `Lists all branches that have saved work-in-progress (WIP).

When you switch branches with pending changes, drift automatically saves them
as WIP. Use this command to see which branches have saved work.`,
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

var wipSaveCmd = &cobra.Command{
	Use:   "save",
	Short: "Save current uncommitted work as WIP",
	Long: `Save current uncommitted work (staged + unstaged) as WIP for the current branch.

This is useful when you want to save your work without committing it, similar
to 'git stash'. The index is cleared after saving. Use 'drift wip restore'
to recover the saved work.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		branch := sharedRepo.CurrentBranch()
		if err := sharedRepo.WIPSave(); err != nil {
			return err
		}
		fmt.Printf("Saved work-in-progress for branch %s\n", branch)
		return nil
	},
}

var wipRestoreCmd = &cobra.Command{
	Use:   "restore [branch]",
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

var wipDropCmd = &cobra.Command{
	Use:   "drop [branch]",
	Short: "Discard saved work-in-progress for a branch",
	Long: `Discards the saved work-in-progress for the current (or specified) branch.
The WIP file is deleted and cannot be recovered.

Use -f to skip the confirmation prompt.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
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

		if !confirmAction(force, fmt.Sprintf("Drop WIP for branch %s?", branch), nil) {
			fmt.Println("Aborted")
			return nil
		}

		if err := sharedRepo.WIPDrop(branch); err != nil {
			return err
		}
		fmt.Printf("Dropped work-in-progress for branch %s\n", branch)
		return nil
	},
}

func init() {
	wipCmd.AddCommand(wipListCmd)
	wipCmd.AddCommand(wipSaveCmd)
	wipCmd.AddCommand(wipRestoreCmd)
	wipCmd.AddCommand(wipDropCmd)
	wipDropCmd.Flags().BoolVarP(&wipDropForce, "force", "f", false, "Skip confirmation prompt")
	rootCmd.AddCommand(wipCmd)
}
