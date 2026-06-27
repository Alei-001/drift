package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewSwitchCmd creates the switch subcommand.
func NewSwitchCmd(application *apppkg.App) *cobra.Command {
	var (
		force  bool
		create bool
	)

	cmd := &cobra.Command{
		Use:   "switch <branch>",
		Short: "Switch to a different branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := args[0]

			result, err := application.Switch(branch, apppkg.SwitchOptions{
				Force:  force,
				Create: create,
			})
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

			if result.Created {
				fmt.Printf("Created and switched to branch %s\n", colorGreen(result.Branch))
			} else {
				fmt.Printf("Switched to branch %s\n", colorGreen(result.Branch))
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force switch (discard changes)")
	cmd.Flags().BoolVarP(&create, "create", "c", false, "Create branch if it doesn't exist")

	return cmd
}
