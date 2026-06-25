package cli

import (
	"fmt"

	"github.com/drift/drift/internal/repo"
	"github.com/spf13/cobra"
)

var switchCmd = &cobra.Command{
	Use:   "switch <branch>",
	Short: "Switch to another branch",
	Long: `Switch to another branch and restore the working tree.
Pending changes are automatically saved and can be recovered with 'drift wip restore'.
Use --force to discard changes without saving.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		branch := args[0]
		force, _ := cmd.Flags().GetBool("force")
		create, _ := cmd.Flags().GetBool("create")

		// Issue 27: no-op if already on this branch.
		currentBranch := sharedRepo.CurrentBranch()
		if branch == currentBranch {
			fmt.Printf("Already on branch: %s\n", branch)
			return nil
		}

		opts := repo.SwitchOptions{
			Force:  force,
			Create: create,
		}

		result, err := sharedRepo.Switch(branch, opts)
		if err != nil {
			return err
		}

		if result.WIPSaved {
			fmt.Printf("Saved pending changes for %s (use 'drift wip restore' to recover)\n", currentBranch)
		}
		if result.Created {
			fmt.Printf("Created branch: %s\n", branch)
		}
		fmt.Printf("Switched to branch: %s\n", branch)
		return nil
	},
}

func init() {
	switchCmd.Flags().Bool("force", false, "Discard pending changes without saving to WIP")
	switchCmd.Flags().BoolP("create", "c", false, "Create the branch if it does not exist, then switch")
	rootCmd.AddCommand(switchCmd)
}
