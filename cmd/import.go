package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Alei-001/drift/internal/porcelain"
)

var importCmd = &cobra.Command{
	Use:   "import <branch> <file>",
	Short: "Import a file from another branch",
	Long: `Import a single file from another branch's latest snapshot into the
current workspace. This is a non-merge file-level cherry-pick: it does not
touch any other workspace files, does not move HEAD, and does not create a
snapshot. Useful for bringing a single file from an experimental branch
into the current branch without switching.

After importing, run 'drift save' to record the change as a new snapshot.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		store, cfg, err := openProjectOrReport("Import", "import", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		branchName := args[0]
		filePath := args[1]

		entry, err := porcelain.ImportFileFromBranch(ctx, store, cwd, branchName, filePath, &cfg.Core)
		if err != nil {
			hint := "use 'drift branch list' to see available branches."
			if errors.Is(err, porcelain.ErrFileNotFound) {
				hint = fmt.Sprintf("use 'drift show branch:%s' to list files in this branch.", branchName)
			}
			reportFailed("Import", "import", "import failed.", hint)
			return ErrSilent
		}

		if globalJSON {
			return outputJSON(JSONEnvelope{
				Command: "import",
				Status:  "ok",
				Data: map[string]any{
					"branch": branchName,
					"file":   entry.Path,
					"size":   entry.Size,
				},
			})
		}

		if globalQuiet {
			return nil
		}

		fmt.Printf(">>> Imported [ok]\n")
		fmt.Println()
		fmt.Printf("  %s  (from branch %s, %s)\n", entry.Path, branchName, formatSize(entry.Size))
		fmt.Println()
		fmt.Println("  1 file imported. Use 'drift save' to record this change.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(importCmd)
}
