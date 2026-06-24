package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	branchDelete bool
	branchMove   string
)

var branchCmd = &cobra.Command{
	Use:   "branch [name]",
	Short: "List, create, delete, or rename branches",
	Long: `List all branches, or create a new branch from the current version.

Without arguments, lists all branches.
With a name, creates a new branch pointing to the current commit.
Use -d to delete a branch, -m to rename a branch.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch {
		case branchDelete:
			if len(args) == 0 {
				return fmt.Errorf("branch name required for -d")
			}
			return deleteBranch(args[0])
		case branchMove != "":
			if len(args) == 0 {
				return fmt.Errorf("branch name required for -m")
			}
			return renameBranch(args[0], branchMove)
		case len(args) == 0 || args[0] == "list":
			return listBranches()
		default:
			return createBranch(args[0])
		}
	},
}

func init() {
	branchCmd.Flags().BoolVarP(&branchDelete, "delete", "d", false, "Delete a branch")
	branchCmd.Flags().StringVarP(&branchMove, "move", "m", "", "Rename a branch")
	rootCmd.AddCommand(branchCmd)
}

func listBranches() error {
	currentBranch := sharedRepo.CurrentBranch()

	names, err := sharedRepo.ListBranches()
	if err != nil {
		return fmt.Errorf("failed to list branches: %w", err)
	}

	if len(names) == 0 {
		fmt.Println("No branches yet")
		return nil
	}

	for _, name := range names {
		prefix := "  "
		if name == currentBranch {
			prefix = "* "
		}
		fmt.Printf("%s%s\n", prefix, name)
	}

	return nil
}

func createBranch(name string) error {
	if err := sharedRepo.CreateBranch(name); err != nil {
		return err
	}
	fmt.Printf("Created branch: %s\n", name)
	return nil
}

// deleteBranch removes a branch ref. It refuses to delete the current branch
// or HEAD. Mirrors `git branch -d`.
func deleteBranch(name string) error {
	if err := sharedRepo.DeleteBranch(name); err != nil {
		return err
	}
	fmt.Printf("Deleted branch: %s\n", name)
	return nil
}

// renameBranch renames a branch. HEAD is updated if it pointed at the old
// name. Mirrors `git branch -m`.
func renameBranch(oldName, newName string) error {
	if err := sharedRepo.RenameBranch(oldName, newName); err != nil {
		return err
	}
	fmt.Printf("Renamed branch: %s → %s\n", oldName, newName)
	return nil
}
