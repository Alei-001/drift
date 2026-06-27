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
		tag     string
	)

	cmd := &cobra.Command{
		Use:   "save",
		Short: "Save staged changes to the repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := application.Save(message, apppkg.SaveOptions{
				Amend: amend,
				All:   all,
				Tag:   tag,
			})
			if err != nil {
				return err
			}

			if result.Amended {
				fmt.Printf("Amended: %s (%s)\n", colorYellow(result.ID), result.Message)
			} else {
				fmt.Printf("Saved: %s (%s)\n", colorGreen(result.ID), result.Message)
			}

			if len(result.ChangedPaths) > 0 {
				fmt.Println(colorGreen(fmt.Sprintf("Changed %d file(s)", len(result.ChangedPaths))))
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Commit message")
	cmd.Flags().BoolVar(&amend, "amend", false, "Amend the last commit")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "Stage all changes before saving")
	cmd.Flags().StringVar(&tag, "tag", "", "Tag this version")

	return cmd
}
