package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	"github.com/spf13/cobra"
)

// Work-in-progress (WIP) auto-save: when switching branches with pending
// changes, the changes are automatically saved to .drift/wip/<branch>.json
// so the user never loses work. This is a friendly alternative to Git's
// stash — no explicit stash command needed, switch just works.
//
// Storage: .drift/wip/<branch>.json — a serialized index of staged entries.

const wipDir = "wip"

// wipEntry stores a single staged file's metadata for WIP recovery.
type wipEntry struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Mode uint32 `json:"mode"`
}

// wipData is the serialized WIP state for a branch.
type wipData struct {
	Branch  string    `json:"branch"`
	Entries []wipEntry `json:"entries"`
}

// saveWIP saves the current index entries to a WIP file for the given branch.
func saveWIP(branch string, idx *core.Index) error {
	wip := wipData{Branch: branch}
	for _, e := range idx.Entries {
		wip.Entries = append(wip.Entries, wipEntry{
			Path: e.Path,
			Hash: e.Hash,
			Mode: e.Mode,
		})
	}

	dir := filepath.Join(sharedStore.DriftDir(), wipDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, branch+".json")
	data, err := json.MarshalIndent(wip, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// loadWIP loads the WIP data for a branch. Returns nil if no WIP exists.
func loadWIP(store *storage.Store, branch string) (*wipData, error) {
	path := filepath.Join(store.DriftDir(), wipDir, branch+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var wip wipData
	if err := json.Unmarshal(data, &wip); err != nil {
		return nil, err
	}
	return &wip, nil
}

// deleteWIP removes the WIP file for a branch.
func deleteWIP(store *storage.Store, branch string) error {
	path := filepath.Join(store.DriftDir(), wipDir, branch+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// stageWorktreeChanges re-stages worktree modifications into the index,
// so they can be captured by saveWIP. This is a simplified version of
// `drift add .` that only updates entries for files that changed.
func stageWorktreeChanges(idx *core.Index) error {
	// Walk the working tree and update entries for modified files.
	parentHashes := loadParentTreeHashes(sharedStore)

	return core.WalkWorkingDir(sharedDir, func(path string, info os.FileInfo) error {
		fullPath := filepath.Join(sharedDir, filepath.FromSlash(path))

		mode, err := core.NormalizeModeForPath(info.Mode(), path)
		if err != nil {
			return nil // skip unsupported types
		}

		var hash string
		if mode == core.ModeSymlink {
			target, err := os.Readlink(fullPath)
			if err != nil {
				return nil
			}
			if err := core.ValidateSymlinkTarget(sharedDir, path, target); err != nil {
				return nil
			}
			hash, err = sharedStore.PutBlob([]byte(target))
			if err != nil {
				return nil
			}
		} else {
			hash, err = putBlobForAdd(sharedStore, fullPath, sharedConfig.Core.AutoCRLF)
			if err != nil {
				return nil
			}
		}

		// Skip if unchanged from parent.
		if parentHash, ok := parentHashes[path]; ok && parentHash == hash {
			return nil
		}

		entry := core.IndexEntry{
			Path:       path,
			Hash:       hash,
			ModifiedAt: info.ModTime(),
			Size:       info.Size(),
			Mode:       mode,
		}
		return idx.Add(entry)
	})
}

var restoreWIPCmd = &cobra.Command{
	Use:   "restore-wip [branch]",
	Short: "Restore work-in-progress saved during a branch switch",
	Long: `Restores the auto-saved work-in-progress for the current (or specified) branch.
When you switch branches with pending changes, drift automatically saves them.
Use this command to restore that work.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		branch := currentBranchName(sharedStore)
		if len(args) > 0 {
			branch = args[0]
		}

		wip, err := loadWIP(sharedStore, branch)
		if err != nil {
			return err
		}
		if wip == nil || len(wip.Entries) == 0 {
			fmt.Printf("No saved work-in-progress for branch %s\n", branch)
			return nil
		}

		// Load the current index and merge WIP entries.
		var idx core.Index
		if err := sharedStore.LoadIndex(&idx); err != nil {
			return err
		}

		var restored int
		for _, e := range wip.Entries {
			entry := core.IndexEntry{
				Path: e.Path,
				Hash: e.Hash,
				Mode: e.Mode,
			}
			if err := idx.Add(entry); err != nil {
				continue
			}
			// Also restore the file content to the worktree.
			blob := core.BlobEntry{Path: e.Path, Hash: e.Hash, Mode: e.Mode}
			if _, err := writeBlobToWorktree(sharedStore, sharedDir, blob); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not restore %s: %v\n", e.Path, err)
				continue
			}
			restored++
		}

		if err := sharedStore.SaveIndex(&idx); err != nil {
			return err
		}

		// Clear the WIP after successful restore.
		_ = deleteWIP(sharedStore, branch)

		fmt.Printf("Restored %d file(s) from work-in-progress for %s\n", restored, branch)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(restoreWIPCmd)
}
