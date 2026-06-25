package cli

import (
	"fmt"
	"sort"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/worktree"
	"github.com/spf13/cobra"
)

var unstageCmd = &cobra.Command{
	Use:   "unstage [<path>...]",
	Short: "Unstage staged changes",
	Long: `Unstage staged changes.

Without arguments, clears the entire staging area.
With path arguments, removes only the matching files from the staging area.
Paths support globs and directories, just like 'drift add'.

Examples:
  drift unstage             # clear the entire staging area
  drift unstage note.txt    # unstage a single file
  drift unstage a.txt b.txt # unstage multiple files
  drift unstage docs/       # unstage all files under a directory
  drift unstage "*.tmp"     # unstage files matching a glob
  drift unstage .           # clear the entire staging area

The working tree files are never modified — only the index is updated.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// No args: clear entire staging area (with confirmation).
		if len(args) == 0 {
			if !confirmAction(false, "Clear entire staging area?", nil) {
				fmt.Println("Aborted")
				return nil
			}
			idx := &core.Index{}
			if err := sharedStore.SaveIndex(idx); err != nil {
				return fmt.Errorf("failed to unstage: %w", err)
			}
			fmt.Println("Staging area cleared")
			return nil
		}

		// Expand glob patterns and collect unique paths (same as add).
		paths, err := worktree.ExpandAddPaths(sharedDir, args)
		if err != nil {
			return err
		}

		// Special case: "." means clear all (same as no args).
		if len(paths) == 1 && paths[0] == "." {
			if !confirmAction(false, "Clear entire staging area?", nil) {
				fmt.Println("Aborted")
				return nil
			}
			idx := &core.Index{}
			if err := sharedStore.SaveIndex(idx); err != nil {
				return fmt.Errorf("failed to unstage: %w", err)
			}
			fmt.Println("Staging area cleared")
			return nil
		}

		// Validate expanded paths (reject traversal, null bytes, etc.).
		for _, p := range paths {
			if err := core.ValidateTreePath(p); err != nil {
				return fmt.Errorf("invalid path: %w", err)
			}
		}

		var idx core.Index
		if err := sharedStore.LoadIndex(&idx); err != nil {
			return fmt.Errorf("failed to load index: %w", err)
		}

		// Find index entries matching the given paths. A path matches if
		// it equals an entry path or is a parent directory of one (so
		// directory arguments unstage all files beneath them).
		var toRemove []string
		for _, entry := range idx.Entries {
			if worktree.PathMatchesAny(entry.Path, paths) {
				toRemove = append(toRemove, entry.Path)
			}
		}
		sort.Strings(toRemove)

		// Report user-specified paths that didn't match any staged entry.
		matched := make(map[string]bool)
		for _, entryPath := range toRemove {
			for _, p := range paths {
				if worktree.PathMatchesAny(entryPath, []string{p}) {
					matched[p] = true
				}
			}
		}
		for _, p := range paths {
			if !matched[p] {
				fmt.Printf("%s is not staged\n", p)
			}
		}

		// Remove matching entries.
		for _, p := range toRemove {
			idx.Remove(p)
			fmt.Printf("Unstaged: %s\n", p)
		}

		if len(toRemove) > 0 {
			if err := sharedStore.SaveIndex(&idx); err != nil {
				return fmt.Errorf("failed to unstage: %w", err)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(unstageCmd)
}
