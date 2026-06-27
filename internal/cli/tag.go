package cli

import (
	"fmt"
	"strings"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewTagCmd creates the tag subcommand group.
func NewTagCmd(application *apppkg.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag <subcommand>",
		Short: "Manage version tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("use 'drift tag list', 'drift tag add <version> <name>', or 'drift tag remove <name>'")
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := application.TagList()
			if err != nil {
				return err
			}
			for _, e := range entries {
				msg := strings.SplitN(e.Message, "\n", 2)[0]
				fmt.Printf("%s  → %s  %s\n", colorYellow(e.Label), colorGreen(e.ID), msg)
			}
			return nil
		},
	}

	addCmd := &cobra.Command{
		Use:   "add <version> <name>",
		Short: "Add a tag to a version",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := application.TagAdd(args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("Tagged %s as '%s'\n", args[0], colorGreen(args[1]))
			return nil
		},
	}

	removeCmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := application.TagDelete(args[0]); err != nil {
				return err
			}
			fmt.Printf("Deleted tag %s\n", colorGreen(args[0]))
			return nil
		},
	}

	cmd.AddCommand(listCmd, addCmd, removeCmd)
	return cmd
}
