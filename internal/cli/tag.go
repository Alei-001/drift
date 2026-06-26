package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewTagCmd creates the tag subcommand.
func NewTagCmd(application *apppkg.App) *cobra.Command {
	var (
		deleteTag string
	)

	cmd := &cobra.Command{
		Use:   "tag [<version>] [<label>]",
		Short: "Manage tags for commits",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Delete tag
			if deleteTag != "" {
				if err := application.TagDelete(deleteTag); err != nil {
					return err
				}
				fmt.Printf("Deleted tag %s\n", deleteTag)
				return nil
			}

			// List tags: drift tag (no args), or drift tag list
			if len(args) == 0 || (len(args) == 1 && args[0] == "list") {
				entries, err := application.TagList()
				if err != nil {
					return err
				}

				if len(entries) == 0 {
					fmt.Println("No tags defined")
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

			// Add tag
			if len(args) == 2 {
				version := args[0]
				label := args[1]
				if err := application.TagAdd(version, label); err != nil {
					return err
				}
				fmt.Printf("Tagged %s as '%s'\n", version, label)
				return nil
			}

			return fmt.Errorf("usage: drift tag <version> <label> or drift tag [list]")
		},
	}

	cmd.Flags().StringVar(&deleteTag, "delete", "", "Delete a tag")

	return cmd
}
