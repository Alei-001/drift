package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Alei-001/drift/internal/porcelain"
)

var exportOutput string

var exportCmd = &cobra.Command{
	Use:   "export <version>",
	Short: "Export a snapshot as a zip archive",
	Long: `Export all files from a snapshot into a zip archive. This is useful
for sharing a specific version of your project with someone who does not
have drift installed.

The version argument accepts the same syntax as other commands:
  head, id:<prefix>, tag:<name>, branch:<name>, <bare-name>

Use -o to specify the output path. If omitted, the archive is written to
drift-export-<short-id>.zip in the current directory.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Export", "export", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		snapshot := resolveSnapshot(ctx, store, args[0])
		if snapshot == nil {
			reportFailed("Export", "export", fmt.Sprintf("snapshot '%s' not found.", args[0]), "use 'drift log' to list available snapshots.")
			return ErrSilent
		}

		output := exportOutput
		if output == "" {
			output = fmt.Sprintf("drift-export-%s.zip", snapshot.ShortID())
		}
		if !filepath.IsAbs(output) {
			if abs, err := filepath.Abs(output); err == nil {
				output = abs
			}
		}

		result, err := porcelain.ExportSnapshot(ctx, store, snapshot.ID, output)
		if err != nil {
			reportFailed("Export", "export", "export failed.", "")
			return ErrSilent
		}

		if globalJSON {
			return outputJSON(JSONEnvelope{
				Command: "export",
				Status:  "ok",
				Data: map[string]any{
					"version":    args[0],
					"output":     output,
					"file_count": result.FileCount,
					"total_size": result.TotalSize,
				},
			})
		}

		if globalQuiet {
			return nil
		}

		fmt.Printf(">>> Exported [ok]\n")
		fmt.Println()
		fmt.Printf("  %s\n", output)
		fmt.Println()
		fmt.Printf("  %d files, %s\n", result.FileCount, formatSize(result.TotalSize))
		return nil
	},
}

func init() {
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "output zip file path (default: drift-export-<short-id>.zip)")
	rootCmd.AddCommand(exportCmd)
}
