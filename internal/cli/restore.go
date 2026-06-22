package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	"github.com/spf13/cobra"
)

var restoreCmd = &cobra.Command{
	Use:   "restore <version>",
	Short: "Restore working tree to a specific version",
	Long: `Restore the working tree to the state of a given version.
Files that differ from the target version will be overwritten.
Untracked files are preserved.
Use --force to discard staged changes.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		version := args[0]
		force, _ := cmd.Flags().GetBool("force")

		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		store := storage.NewStore(dir)
		if !store.IsInitialized() {
			return fmt.Errorf("not a drift project (run 'drift init')")
		}

		var oldIdx core.Index
		if err := store.LoadIndex(&oldIdx); err != nil {
			return err
		}
		if len(oldIdx.Entries) > 0 && !force {
			return fmt.Errorf("staging area has pending changes (use --force to discard)")
		}

		commit, err := findCommitByPrefix(store, version)
		if err != nil {
			return err
		}

		targetTree, err := store.GetTree(commit.TreeHash)
		if err != nil {
			return fmt.Errorf("failed to load target tree: %w", err)
		}

		reader := core.NewTreeReader(store)
		targetBlobs, err := reader.ListBlobs(targetTree, "")
		if err != nil {
			return err
		}

		targetPaths := make(map[string]bool)
		for _, b := range targetBlobs {
			targetPaths[b.Path] = true
		}

		prevBlobs := make(map[string]bool)
		allCommits, err := store.ListCommits()
		if err == nil && len(allCommits) > 0 {
			latest := allCommits[len(allCommits)-1]
			if latest.Hash != commit.Hash {
				if t, err := store.GetTree(latest.TreeHash); err == nil {
					prevBlobsList, _ := reader.ListBlobs(t, "")
					for _, b := range prevBlobsList {
						prevBlobs[b.Path] = true
					}
				}
			}
		}

		newIdx := &core.Index{}
		var added, modified, deleted int

		for _, b := range targetBlobs {
			fullPath := filepath.Join(dir, filepath.FromSlash(b.Path))
			data, err := store.GetBlob(b.Hash)
			if err != nil {
				return err
			}

			existing, err := os.ReadFile(fullPath)
			if err == nil && string(existing) == string(data) {
				info, _ := os.Stat(fullPath)
				newIdx.Add(core.IndexEntry{
					Path:       b.Path,
					Hash:       b.Hash,
					ModifiedAt: info.ModTime(),
					Size:       info.Size(),
					Mode:       b.Mode,
				})
				continue
			}

			if err != nil {
				added++
			} else {
				modified++
			}

			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(fullPath, data, os.FileMode(b.Mode)); err != nil {
				return err
			}

			info, _ := os.Stat(fullPath)
			newIdx.Add(core.IndexEntry{
				Path:       b.Path,
				Hash:       b.Hash,
				ModifiedAt: info.ModTime(),
				Size:       info.Size(),
				Mode:       b.Mode,
			})
		}

		for path := range prevBlobs {
			if !targetPaths[path] {
				fullPath := filepath.Join(dir, filepath.FromSlash(path))
				if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
					return err
				}
				deleted++
			}
		}

		for _, entry := range oldIdx.Entries {
			if !targetPaths[entry.Path] {
				if _, inPrev := prevBlobs[entry.Path]; !inPrev {
					fullPath := filepath.Join(dir, filepath.FromSlash(entry.Path))
					if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
						return err
					}
					deleted++
				}
			}
		}

		cleanEmptyDirs(dir, newIdx)

		if err := store.SaveIndex(newIdx); err != nil {
			return fmt.Errorf("failed to update index: %w", err)
		}

		fmt.Printf("Restored to %s: %d added, %d modified, %d deleted\n", version, added, modified, deleted)
		return nil
	},
}

func init() {
	restoreCmd.Flags().Bool("force", false, "Discard staged changes and force restore")
	rootCmd.AddCommand(restoreCmd)
}

func cleanEmptyDirs(root string, idx *core.Index) {
	tracked := make(map[string]bool)
	for _, e := range idx.Entries {
		dir := filepath.Dir(e.Path)
		for dir != "." {
			tracked[dir] = true
			dir = filepath.Dir(dir)
		}
	}

	var dirs []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." || rel == ".drift" || len(rel) > 0 && rel[0] == '.' {
			return nil
		}
		dirs = append(dirs, rel)
		return nil
	})

	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})

	for _, d := range dirs {
		if !tracked[filepath.ToSlash(d)] {
			os.Remove(filepath.Join(root, d))
		}
	}
}
