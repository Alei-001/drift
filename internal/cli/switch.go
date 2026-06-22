package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	"github.com/spf13/cobra"
)

var switchCmd = &cobra.Command{
	Use:   "switch <branch>",
	Short: "Switch to another branch",
	Long: `Switch to another branch and restore the working tree.
The staging area must be empty (or use --force to discard).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		branch := args[0]
		force, _ := cmd.Flags().GetBool("force")

		var idx core.Index
		if err := sharedStore.LoadIndex(&idx); err != nil {
			return err
		}
		if len(idx.Entries) > 0 && !force {
			return fmt.Errorf("staging area has pending changes (use --force to discard)")
		}

		commitHash, err := sharedStore.GetRef(branch)
		if err != nil {
			return fmt.Errorf("branch not found: %s", branch)
		}

		reader := core.NewTreeReader(sharedStore)

		currentBlobs := make(map[string]bool)
		commits, _ := sharedStore.ListCommits()
		if len(commits) > 0 {
			latest := commits[len(commits)-1]
			if t, err := sharedStore.GetTree(latest.TreeHash); err == nil {
				if blobs, err := reader.ListBlobs(t, ""); err == nil {
					for _, b := range blobs {
						currentBlobs[b.Path] = true
					}
				}
			}
		}

		if err := sharedStore.SaveRef("HEAD", branch); err != nil {
			return fmt.Errorf("failed to update HEAD: %w", err)
		}

		commit, err := findCommitByHash(sharedStore, commitHash)
		if err != nil {
			return fmt.Errorf("failed to load commit: %w", err)
		}

		targetTree, err := sharedStore.GetTree(commit.TreeHash)
		if err != nil {
			return fmt.Errorf("failed to load tree: %w", err)
		}

		targetBlobs, err := reader.ListBlobs(targetTree, "")
		if err != nil {
			return err
		}

		targetPaths := make(map[string]bool)
		for _, b := range targetBlobs {
			targetPaths[b.Path] = true
		}

		newIdx := &core.Index{}
		for _, b := range targetBlobs {
			fullPath := filepath.Join(sharedDir, filepath.FromSlash(b.Path))
			data, err := sharedStore.GetBlob(b.Hash)
			if err != nil {
				return err
			}

			existing, err := os.ReadFile(fullPath)
			if err == nil && bytes.Equal(existing, data) {
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

			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(fullPath, data, os.FileMode(core.ToOSFileMode(b.Mode))); err != nil {
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

		for path := range currentBlobs {
			if !targetPaths[path] {
				fullPath := filepath.Join(sharedDir, filepath.FromSlash(path))
				if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
		}

		cleanEmptyDirs(sharedDir, newIdx)

		if err := sharedStore.SaveIndex(newIdx); err != nil {
			return fmt.Errorf("failed to update index: %w", err)
		}

		fmt.Printf("Switched to branch: %s\n", branch)
		return nil
	},
}

func init() {
	switchCmd.Flags().Bool("force", false, "Discard staged changes and force switch")
	rootCmd.AddCommand(switchCmd)
}

func findCommitByHash(store *storage.Store, hash string) (*core.Commit, error) {
	commits, err := store.ListCommits()
	if err != nil {
		return nil, err
	}

	for _, c := range commits {
		if c.Hash == hash {
			return c, nil
		}
	}

	return nil, fmt.Errorf("commit not found: %s", hash)
}
