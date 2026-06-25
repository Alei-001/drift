package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewConfigCmd creates the config subcommand.
func NewConfigCmd(application *apppkg.App) *cobra.Command {
	var (
		global  bool
		list    bool
		unset   string
	)

	cmd := &cobra.Command{
		Use:   "config [<key> [<value>]]",
		Short: "Get or set configuration options",
		RunE: func(cmd *cobra.Command, args []string) error {
			scope := apppkg.LocalScope
			if global {
				scope = apppkg.GlobalScope
			}

			// List all config
			if list {
				entries, err := application.ConfigList(scope)
				if err != nil {
					return err
				}

				for _, e := range entries {
					fmt.Printf("%s = %s\n", e.Key, e.Value)
				}
				return nil
			}

			// Unset config
			if unset != "" {
				if err := application.ConfigUnset(scope, unset); err != nil {
					return err
				}
				fmt.Printf("Unset %s\n", unset)
				return nil
			}

			// Get config
			if len(args) == 1 {
				value, err := application.ConfigGet(scope, args[0])
				if err != nil {
					return err
				}
				fmt.Println(value)
				return nil
			}

			// Set config
			if len(args) == 2 {
				key := args[0]
				value := args[1]
				if err := application.ConfigSet(scope, key, value); err != nil {
					return err
				}
				fmt.Printf("Set %s = %s\n", key, value)
				return nil
			}

			return cmd.Help()
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "Use global config")
	cmd.Flags().BoolVar(&list, "list", false, "List all config options")
	cmd.Flags().StringVar(&unset, "unset", "", "Unset a config option")

	return cmd
}
