package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewWIPCmd creates the wip subcommand.
func NewWIPCmd(application *apppkg.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wip <subcommand>",
		Short: "Manage work in progress",
	}

	// wip list
	listCmd := &cobra.Command{
		Use:   "list [branch]",
		Short: "List work in progress",
		RunE: func(cmd *cobra.Command, args []string) error {
			var branch string
			if len(args) > 0 {
				branch = args[0]
			}

			// If no branch specified, list all branches with WIP
			if branch == "" {
				branches, err := application.WIPListAll()
				if err != nil {
					return err
				}

				if len(branches) == 0 {
					fmt.Println(colorGray("No work in progress"))
					return nil
				}

				for _, b := range branches {
					fmt.Println(colorCyan(b))
				}
				return nil
			}

			// List WIP for specific branch
			entries, err := application.WIPList(branch)
			if err != nil {
				return err
			}

			if len(entries) == 0 {
				fmt.Println(colorGray(fmt.Sprintf("No work in progress for branch %s", branch)))
				return nil
			}

			for _, e := range entries {
				h := e.Hash
				if len(h) > 8 {
					h = h[:8]
				}
				fmt.Printf("%s %s\n", e.Path, colorYellow(h))
			}
			return nil
		},
	}

	// wip save
	saveCmd := &cobra.Command{
		Use:   "save",
		Short: "Save work in progress",
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := application.CurrentBranch()
			count, err := application.WIPSave(branch)
			if err != nil {
				return err
			}
			if count == 0 {
				fmt.Println(colorGray("Nothing to save"))
				return nil
			}
			fmt.Println(colorGreen(fmt.Sprintf("Saved work in progress for branch %s (%d file(s))", branch, count)))
			return nil
		},
	}

	// wip restore
	restoreCmd := &cobra.Command{
		Use:   "restore [branch]",
		Short: "Restore work in progress",
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := application.CurrentBranch()
			if len(args) > 0 {
				branch = args[0]
			}

			count, err := application.WIPRestore(branch)
			if err != nil {
				return err
			}

			if count == 0 {
				fmt.Println(colorGray(fmt.Sprintf("No work in progress for branch %s", branch)))
				return nil
			}
			fmt.Println(colorGreen(fmt.Sprintf("Restored %d file(s) from work in progress for branch %s", count, branch)))
			return nil
		},
	}

	// wip drop
	dropCmd := &cobra.Command{
		Use:   "drop [branch]",
		Short: "Drop work in progress",
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := application.CurrentBranch()
			if len(args) > 0 {
				branch = args[0]
			}

			if !confirmAction(false, fmt.Sprintf("Drop work in progress for branch %s?", branch), nil) {
				fmt.Println(colorRed("Aborted"))
				return nil
			}

			if err := application.WIPDrop(branch); err != nil {
				return err
			}

			fmt.Println(colorGreen(fmt.Sprintf("Dropped work in progress for branch %s", branch)))
			return nil
		},
	}

	cmd.AddCommand(listCmd, saveCmd, restoreCmd, dropCmd)

	return cmd
}
