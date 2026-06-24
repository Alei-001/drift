package cli

import (
	"fmt"

	"github.com/drift/drift/internal/repo"
	"github.com/spf13/cobra"
)

var saveCmd = &cobra.Command{
	Use:   "save [-m message] [--amend] [--all] [--name label]",
	Short: "Save staged changes as a new version",
	RunE: func(cmd *cobra.Command, args []string) error {
		message, _ := cmd.Flags().GetString("message")
		amend, _ := cmd.Flags().GetBool("amend")
		all, _ := cmd.Flags().GetBool("all")
		nameLabel, _ := cmd.Flags().GetString("name")

		// Validate name label early so we fail before saving.
		if nameLabel != "" {
			if err := repo.ValidateNameLabel(nameLabel); err != nil {
				return err
			}
		}

		opts := repo.SaveOptions{
			Message: message,
			Amend:   amend,
			All:     all,
			Name:    nameLabel,
		}

		result, err := sharedRepo.Save(opts)
		if err != nil {
			return err
		}

		if result.Amended {
			fmt.Printf("Amended version %s: %s\n", result.ID, result.Message)
		} else if result.Message != "" {
			fmt.Printf("Saved version %s: %s\n", result.ID, result.Message)
		} else {
			fmt.Printf("Saved version %s\n", result.ID)
		}

		fmt.Printf("\n  %d file(s) saved:\n", len(result.StagedPaths))
		for _, p := range result.StagedPaths {
			fmt.Printf("    %s\n", p)
		}

		// Print name assignment if a name was provided.
		if nameLabel != "" {
			fmt.Printf("Named %s as '%s'\n", result.ID, nameLabel)
		}

		// Auto-sync after a successful save (no-op if sync is disabled).
		AutoSyncAfterSave(sharedDir, sharedConfig, sharedStore)
		return nil
	},
}

func init() {
	saveCmd.Flags().StringP("message", "m", "", "Version message")
	saveCmd.Flags().Bool("amend", false, "Amend the most recent version instead of creating a new one")
	saveCmd.Flags().BoolP("all", "a", false, "Automatically stage all changes before saving")
	saveCmd.Flags().String("name", "", "Assign a version name (alias) to this version")
	rootCmd.AddCommand(saveCmd)
}
