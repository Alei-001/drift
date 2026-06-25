package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewSaveCmd creates the save subcommand.
func NewSaveCmd(application *apppkg.App) *cobra.Command {
	var (
		message string
		amend   bool
		all     bool
		name    string
	)

	cmd := &cobra.Command{
		Use:   "save",
		Short: "Save staged changes to the repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := application.Save(message, apppkg.SaveOptions{
				Amend: amend,
				All:   all,
				Name:  name,
			})
			if err != nil {
				return err
			}

			if result.Amended {
				fmt.Printf("Amended: %s (%s)\n", result.ID, result.Message)
			} else {
				fmt.Printf("Saved: %s (%s)\n", result.ID, result.Message)
			}

			if len(result.StagedPaths) > 0 {
				fmt.Printf("Staged %d file(s)\n", len(result.StagedPaths))
			}

			// AutoSync after save (best-effort)
			if err := application.AutoSync(); err != nil {
				fmt.Printf("Warning: sync failed: %v\n", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Commit message")
	cmd.Flags().BoolVar(&amend, "amend", false, "Amend the last commit")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "Stage all changes before saving")
	cmd.Flags().StringVar(&name, "name", "", "Name this version")

	return cmd
}
