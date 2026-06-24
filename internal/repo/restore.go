package repo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/worktree"
)

// RestoreResult contains the outcome of a restore operation.
type RestoreResult struct {
	Version  string
	Added    int
	Modified int
	Deleted  int
}

// Restore restores the working tree to a specific version.
// If filters is non-empty, only files matching the filters are restored.
func (r *Repository) Restore(version string, filters []string, force bool) (*RestoreResult, error) {
	hasFilter := len(filters) > 0

	var oldIdx core.Index
	if err := r.Store.LoadIndex(&oldIdx); err != nil {
		return nil, err
	}

	if !force {
		hasPending, err := r.HasPendingStagedChanges(&oldIdx, filters)
		if err != nil {
			return nil, fmt.Errorf("failed to check pending staged changes: %w", err)
		}
		if hasPending {
			return nil, fmt.Errorf("staging area has pending changes (use --force to discard)")
		}
		currentCommit, err := r.CurrentCommit()
		if err != nil {
			return nil, fmt.Errorf("failed to load current commit: %w", err)
		}
		dirty, err := r.WT.HasModifications(currentCommit, &oldIdx, filters)
		if err != nil {
			return nil, fmt.Errorf("failed to check worktree modifications: %w", err)
		}
		if dirty {
			return nil, fmt.Errorf("working tree has unstaged modifications (use --force to discard)")
		}
	}

	commit, err := r.ResolveCommit(version)
	if err != nil {
		return nil, err
	}

	targetTree, err := r.Store.GetTree(commit.TreeHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load target tree: %w", err)
	}

	reader := core.NewTreeReader(r.Store)
	targetBlobs, err := reader.ListBlobs(targetTree, "")
	if err != nil {
		return nil, err
	}

	if hasFilter {
		targetBlobs = worktree.FilterBlobs(targetBlobs, filters)
		if len(targetBlobs) == 0 {
			return nil, fmt.Errorf("no matching files found in %s for given paths", version)
		}
	}

	targetPaths := make(map[string]bool)
	for _, b := range targetBlobs {
		targetPaths[b.Path] = true
	}

	prevBlobs := make(map[string]bool)
	currentBranch := r.CurrentBranch()
	if currentHash, err := r.Store.GetRef(currentBranch); err == nil {
		if currentHash != commit.Hash {
			if currentCommit, err := r.FindCommitByHash(currentHash); err == nil {
				if t, err := r.Store.GetTree(currentCommit.TreeHash); err == nil {
					prevBlobsList, _ := reader.ListBlobs(t, "")
					for _, b := range prevBlobsList {
						prevBlobs[b.Path] = true
					}
				}
			}
		}
	}

	newIdx := &core.Index{}
	if hasFilter {
		for _, e := range oldIdx.Entries {
			if !worktree.PathMatchesAny(e.Path, filters) {
				newIdx.Add(e)
			}
		}
	}

	var deletedPaths []string

	for _, b := range targetBlobs {
		entry, err := r.WT.WriteBlob(b)
		if err != nil {
			return nil, err
		}
		newIdx.Add(entry)
	}

	var added, modified int
	for _, b := range targetBlobs {
		if prevBlobs[b.Path] {
			modified++
		} else {
			added++
		}
	}

	var deleted int
	for path := range prevBlobs {
		if targetPaths[path] {
			continue
		}
		if hasFilter && !worktree.PathMatchesAny(path, filters) {
			continue
		}
		if err := core.ValidateTreePath(path); err != nil {
			continue
		}
		fullPath := filepath.Join(r.Dir, filepath.FromSlash(path))
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		deleted++
		deletedPaths = append(deletedPaths, path)
	}

	for _, entry := range oldIdx.Entries {
		if targetPaths[entry.Path] {
			continue
		}
		if _, inPrev := prevBlobs[entry.Path]; inPrev {
			continue
		}
		if hasFilter && !worktree.PathMatchesAny(entry.Path, filters) {
			continue
		}
		if err := core.ValidateTreePath(entry.Path); err != nil {
			continue
		}
		fullPath := filepath.Join(r.Dir, filepath.FromSlash(entry.Path))
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		deleted++
		deletedPaths = append(deletedPaths, entry.Path)
	}

	r.WT.CleanEmptyDirs(deletedPaths)

	if err := r.Store.SaveIndex(newIdx); err != nil {
		return nil, fmt.Errorf("failed to update index: %w", err)
	}

	return &RestoreResult{
		Version:  version,
		Added:    added,
		Modified: modified,
		Deleted:  deleted,
	}, nil
}
