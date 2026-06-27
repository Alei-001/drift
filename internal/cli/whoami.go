package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

func NewWhoamiCmd(application *apppkg.App) *cobra.Command {
	var local bool

	cmd := &cobra.Command{
		Use:   "whoami [set <name> <email>]",
		Short: "Show or set your identity",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				author := application.Author()
				fmt.Printf("You are %s <%s>\n", colorGreen(author.Name), colorGreen(author.Email))
				return nil
			}
			return fmt.Errorf("use 'drift whoami' to see identity, or 'drift whoami set <name> <email>' to set it")
		},
	}

	setCmd := &cobra.Command{
		Use:   "set <name> <email>",
		Short: "Set your identity",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope := apppkg.GlobalScope
			if local {
				scope = apppkg.LocalScope
			}
			if err := application.ConfigSet(scope, "user.name", args[0]); err != nil {
				return err
			}
			if err := application.ConfigSet(scope, "user.email", args[1]); err != nil {
				return err
			}
			fmt.Println(colorGreen(fmt.Sprintf("Identity set: %s <%s>", args[0], args[1])))
			return nil
		},
	}
	setCmd.Flags().BoolVar(&local, "local", false, "Set for this project only")

	cmd.AddCommand(setCmd)
	return cmd
}
