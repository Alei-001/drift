package cli

import (
	"fmt"
	"os"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewSaveCmd creates the save subcommand.
func NewSaveCmd(application *apppkg.App) *cobra.Command {
	var (
		message string
		tag     string
	)

	cmd := &cobra.Command{
		Use:   "save",
		Short: "Save all working tree changes as a new version",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := application.Save(message, apppkg.SaveOptions{
				Tag: tag,
			})
			if err != nil {
				return err
			}

			name := result.Message
			if name == "" {
				name = "(no message)"
			}
			fmt.Printf("Saved version %s: %s\n", colorYellow(result.ID), name)

			if result.TagWarning != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", colorYellow("Warning"), result.TagWarning)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Commit message")
	cmd.Flags().StringVar(&tag, "tag", "", "Tag this version")

	return cmd
}
