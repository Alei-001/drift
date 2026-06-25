package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewBranchCmd creates the branch subcommand.
func NewBranchCmd(application *apppkg.App) *cobra.Command {
	var (
		deleteBranch string
		moveBranch   string
	)

	cmd := &cobra.Command{
		Use:   "branch [<name>]",
		Short: "List, create, delete, or rename branches",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Delete branch
			if deleteBranch != "" {
				if err := application.BranchDelete(deleteBranch); err != nil {
					return err
				}
				fmt.Printf("Deleted branch %s\n", deleteBranch)
				return nil
			}

			// Rename branch
			if moveBranch != "" {
				if len(args) == 0 {
					return fmt.Errorf("new branch name required")
				}
				newName := args[0]
				if err := application.BranchRename(moveBranch, newName); err != nil {
					return err
				}
				fmt.Printf("Renamed branch %s to %s\n", moveBranch, newName)
				return nil
			}

			// Create branch
			if len(args) > 0 {
				name := args[0]
				if err := application.BranchCreate(name); err != nil {
					return err
				}
				fmt.Printf("Created branch %s\n", name)
				return nil
			}

			// List branches
			branches, err := application.BranchList()
			if err != nil {
				return err
			}

			currentBranch := application.CurrentBranch()
			for _, b := range branches {
				if b == currentBranch {
					fmt.Printf("* %s\n", b)
				} else {
					fmt.Printf("  %s\n", b)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&deleteBranch, "delete", "d", "", "Delete a branch")
	cmd.Flags().StringVarP(&moveBranch, "move", "m", "", "Rename a branch")

	return cmd
}
