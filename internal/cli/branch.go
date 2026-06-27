package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewBranchCmd creates the branch subcommand group.
func NewBranchCmd(application *apppkg.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "branch <subcommand>",
		Short: "Manage branches",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("use 'drift branch list', 'drift branch create <name>', 'drift branch switch <name>', 'drift branch remove <name>', or 'drift branch rename <old> <new>'")
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all branches",
		RunE: func(cmd *cobra.Command, args []string) error {
			branches, err := application.BranchList()
			if err != nil {
				return err
			}
			current := application.CurrentBranch()
			for _, b := range branches {
				if b == current {
					fmt.Printf("* %s\n", colorGreen(b))
				} else {
					fmt.Printf("  %s\n", b)
				}
			}
			return nil
		},
	}

	createCmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new branch and switch to it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := application.BranchCreate(args[0]); err != nil {
				return err
			}
			if _, err := application.Switch(args[0], apppkg.SwitchOptions{}); err != nil {
				return err
			}
			fmt.Printf("Created and switched to branch %s\n", colorGreen(args[0]))
			return nil
		},
	}

	switchCmd := &cobra.Command{
		Use:   "switch <name>",
		Short: "Switch to a branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := application.Switch(args[0], apppkg.SwitchOptions{})
			if err != nil {
				return err
			}

			if result.AlreadyOnBranch {
				fmt.Println(colorGray(fmt.Sprintf("Already on branch %s", result.Branch)))
				return nil
			}

			if result.WIPSaved {
				fmt.Println(colorYellow("Saved work in progress"))
			}

			fmt.Printf("Switched to branch %s\n", colorGreen(result.Branch))
			return nil
		},
	}

	var removeForce bool
	removeCmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Delete a branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !removeForce {
				if !confirmAction(false, fmt.Sprintf("Delete branch %s?", args[0]), nil) {
					fmt.Println(colorRed("Aborted"))
					return nil
				}
			}
			if err := application.BranchDelete(args[0]); err != nil {
				return err
			}
			fmt.Printf("Deleted branch %s\n", colorCyan(args[0]))
			return nil
		},
	}
	removeCmd.Flags().BoolVar(&removeForce, "force", false, "Skip confirmation prompt")

	renameCmd := &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Rename a branch",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := application.BranchRename(args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("Renamed branch %s to %s\n", colorCyan(args[0]), colorCyan(args[1]))
			return nil
		},
	}

	cmd.AddCommand(listCmd, createCmd, switchCmd, removeCmd, renameCmd)
	return cmd
}
