package cli

import (
	"fmt"

	"github.com/drift/drift/internal/worktree"
	"github.com/spf13/cobra"
)

var restoreCmd = &cobra.Command{
	Use:   "restore <version> [<path>...]",
	Short: "Restore working tree to a specific version",
	Long: `Restore the working tree to the state of a given version.
Version can be a version ID (e.g., v1) or branch name (e.g., main).
Files that differ from the target version will be overwritten.
Branch reference is NOT changed - only working tree is updated.
Untracked files are preserved.
Use --force to discard staged changes and unstaged modifications.

If one or more paths are given, only files matching those paths are
restored; all other files are left untouched. This is useful for
reverting a single file or directory without affecting the rest of
the working tree.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		version := args[0]
		force, _ := cmd.Flags().GetBool("force")

		filters, err := worktree.NormalizePathFilters(args[1:])
		if err != nil {
			return err
		}

		result, err := sharedRepo.Restore(version, filters, force)
		if err != nil {
			return err
		}

		fmt.Printf("Restored to %s: %d added, %d modified, %d deleted\n",
			result.Version, result.Added, result.Modified, result.Deleted)
		return nil
	},
}

func init() {
	restoreCmd.Flags().Bool("force", false, "Discard staged changes and unstaged modifications, then force restore")
	rootCmd.AddCommand(restoreCmd)
}
