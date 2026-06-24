package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

var (
	rmCached    bool
	rmRecursive bool
)

var rmCmd = &cobra.Command{
	Use:   "rm <path> [<path>...]",
	Short: "Remove files from the working tree and the staging area",
	Long: `Remove tracked files from the working tree and the staging area.

By default, removes the file from both the index and the working tree.
Use --cached to remove from the index only, keeping the working tree file.

Examples:
  drift rm note.txt          # unstage and delete the file
  drift rm --cached note.txt # unstage but keep the file on disk
  drift rm -r docs/          # remove a directory recursively
  drift rm "*.tmp"           # remove via glob pattern`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, err := expandRmPaths(args, rmRecursive)
		if err != nil {
			return err
		}
		if len(paths) == 0 {
			return fmt.Errorf("no matching files found")
		}

		var idx core.Index
		if err := sharedStore.LoadIndex(&idx); err != nil {
			return fmt.Errorf("failed to load index: %w", err)
		}

		// Build a set of tracked paths (index + last commit) so we can
		// reject untracked files like git does.
		tracked := make(map[string]bool)
		for _, e := range idx.Entries {
			tracked[e.Path] = true
		}
		parentHashes, err := sharedRepo.WT.LoadParentTreeHashes()
		if err != nil {
			return fmt.Errorf("failed to load tracked paths: %w", err)
		}
		for p := range parentHashes {
			tracked[p] = true
		}

		var removed int
		var skipped []string
		for _, p := range paths {
			if !tracked[p] {
				skipped = append(skipped, p)
				continue
			}
			// Remove from index.
			idx.Remove(p)

			if !rmCached {
				// Remove from working tree.
				fullPath := filepath.Join(sharedDir, filepath.FromSlash(p))
				if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("failed to remove %s: %w", p, err)
				}
			}
			removed++
			fmt.Printf("Removed: %s\n", p)
		}

		if removed == 0 {
			if len(skipped) > 0 {
				return fmt.Errorf("pathspec '%s' did not match any tracked files", strings.Join(skipped, "', '"))
			}
			return fmt.Errorf("no tracked files matched")
		}

		if err := sharedStore.SaveIndex(&idx); err != nil {
			return fmt.Errorf("failed to save index: %w", err)
		}

		// Clean up empty directories left behind by removals.
		if !rmCached {
			sharedRepo.WT.CleanEmptyDirs(paths)
		}

		fmt.Printf("Removed %d file(s)\n", removed)
		return nil
	},
}

func init() {
	rmCmd.Flags().BoolVar(&rmCached, "cached", false, "Remove from index only, keep the working tree file")
	rmCmd.Flags().BoolVarP(&rmRecursive, "recursive", "r", false, "Allow recursive removal of directories")
	rootCmd.AddCommand(rmCmd)
}

// expandRmPaths expands glob patterns and directory arguments into a
// deduplicated list of repository-relative file paths. Directories require
// the --recursive flag, mirroring git's safety behavior.
func expandRmPaths(args []string, recursive bool) ([]string, error) {
	seen := make(map[string]bool)
	var result []string

	for _, arg := range args {
		if strings.ContainsAny(arg, "*?[") {
			matches, err := filepath.Glob(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid glob pattern %q: %w", arg, err)
			}
			for _, m := range matches {
				absPath, err := filepath.Abs(m)
				if err != nil {
					absPath = m
				}
				rel, err := filepath.Rel(sharedDir, absPath)
				if err != nil {
					rel = m
				}
				rel = filepath.ToSlash(rel)
				info, err := os.Lstat(m)
				if err != nil {
					continue
				}
				if info.IsDir() {
					if !recursive {
						continue
					}
					collectDirFiles(rel, seen, &result)
					continue
				}
				if !seen[rel] {
					seen[rel] = true
					result = append(result, rel)
				}
			}
			continue
		}

		info, err := os.Lstat(arg)
		if err != nil {
			// Path may have already been deleted from the worktree; still
			// allow removing it from the index if tracked. Use the literal
			// argument as the path.
			rel := filepath.ToSlash(arg)
			if !seen[rel] {
				seen[rel] = true
				result = append(result, rel)
			}
			continue
		}

		absPath, err := filepath.Abs(arg)
		if err != nil {
			absPath = arg
		}
		rel, err := filepath.Rel(sharedDir, absPath)
		if err != nil {
			rel = arg
		}
		rel = filepath.ToSlash(rel)

		if info.IsDir() {
			if !recursive {
				return nil, fmt.Errorf("not removing recursively without -r: %s", rel)
			}
			collectDirFiles(rel, seen, &result)
			continue
		}

		if !seen[rel] {
			seen[rel] = true
			result = append(result, rel)
		}
	}

	return result, nil
}

// collectDirFiles walks a directory and appends file paths to result.
func collectDirFiles(dirRel string, seen map[string]bool, result *[]string) {
	fullDir := filepath.Join(sharedDir, filepath.FromSlash(dirRel))
	_ = core.WalkWorkingDirWithIgnore(fullDir, sharedDir, func(path string, info os.FileInfo) error {
		relPath := filepath.ToSlash(filepath.Join(dirRel, path))
		if !seen[relPath] {
			seen[relPath] = true
			*result = append(*result, relPath)
		}
		return nil
	})
}
