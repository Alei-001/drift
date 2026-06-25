package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewDiffCmd creates the diff subcommand.
func NewDiffCmd(application *apppkg.App) *cobra.Command {
	var (
		version1 string
		version2 string
		patch    bool
		output   string
	)

	cmd := &cobra.Command{
		Use:   "diff [<version1>] [<version2>] [-- <path>...]",
		Short: "Show differences between versions",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse paths after -- separator
			var paths []string
			for i, arg := range args {
				if arg == "--" {
					paths = args[i+1:]
					args = args[:i]
					break
				}
			}

			// Parse version arguments
			if len(args) > 0 {
				version1 = args[0]
			}
			if len(args) > 1 {
				version2 = args[1]
			}

			result, err := application.Diff(apppkg.DiffOptions{
				V1:    version1,
				V2:    version2,
				Paths: paths,
			})
			if err != nil {
				return err
			}

			// Print summary
			fmt.Printf("Changed %d file(s)\n", len(result.Entries))

			return nil
		},
	}

	cmd.Flags().StringVar(&version1, "from", "", "Source version")
	cmd.Flags().StringVar(&version2, "to", "", "Target version")
	cmd.Flags().BoolVarP(&patch, "patch", "p", false, "Show unified diff output")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write patch to file")

	return cmd
}
