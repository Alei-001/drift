package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewExportCmd creates the export subcommand.
func NewExportCmd(application *apppkg.App) *cobra.Command {
	var (
		output string
		format string
	)

	cmd := &cobra.Command{
		Use:   "export <version> [-o <output>] [-f <format>] [<path>...]",
		Short: "Export a snapshot to a directory or archive",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			version := args[0]
			filters := args[1:]

			var exportFormat apppkg.ExportFormat
			switch format {
			case "zip":
				exportFormat = apppkg.ExportZip
			case "tar":
				exportFormat = apppkg.ExportTar
			case "dir", "":
				exportFormat = apppkg.ExportDir
			default:
				return fmt.Errorf("unsupported format: %s (use dir, zip, or tar)", format)
			}

			if actualOutput, err := application.Export(version, output, exportFormat, filters); err != nil {
				return err
			} else {
				fmt.Printf("Exported %s to %s\n", version, actualOutput)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output path (required)")
	cmd.Flags().StringVarP(&format, "format", "f", "dir", "Export format (dir, zip, tar)")
	_ = cmd.MarkFlagRequired("output")

	return cmd
}
