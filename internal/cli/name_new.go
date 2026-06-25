package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewNameCmd creates the name subcommand.
func NewNameCmd(application *apppkg.App) *cobra.Command {
	var (
		listNames bool
		deleteName string
	)

	cmd := &cobra.Command{
		Use:   "name [<version>] [<label>]",
		Short: "Manage named references to commits",
		RunE: func(cmd *cobra.Command, args []string) error {
			// List names
			if listNames {
				entries, err := application.NameList()
				if err != nil {
					return err
				}

				if len(entries) == 0 {
					fmt.Println("No names defined")
					return nil
				}

				for _, e := range entries {
					if e.ID != "" {
						fmt.Printf("%-20s %s %s\n", e.Label, e.ID, e.Message)
					} else {
						fmt.Printf("%-20s %s\n", e.Label, e.Hash[:8])
					}
				}
				return nil
			}

			// Delete name
			if deleteName != "" {
				if err := application.NameDelete(deleteName); err != nil {
					return err
				}
				fmt.Printf("Deleted name %s\n", deleteName)
				return nil
			}

			// Add name
			if len(args) == 2 {
				version := args[0]
				label := args[1]
				if err := application.NameAdd(version, label); err != nil {
					return err
				}
				fmt.Printf("Named %s as '%s'\n", version, label)
				return nil
			}

			return fmt.Errorf("usage: drift name <version> <label> or drift name --list")
		},
	}

	cmd.Flags().BoolVar(&listNames, "list", false, "List all names")
	cmd.Flags().StringVar(&deleteName, "delete", "", "Delete a name")

	return cmd
}
