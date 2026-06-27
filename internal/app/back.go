package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/worktree"
)

type RestoreResult struct {
	Version  string
	Added    int
	Modified int
	Deleted  int
}

func (a *App) Restore(version string, filters []string, force bool) (*RestoreResult, error) {
	hasFilter := len(filters) > 0
	var normalized []string
	if hasFilter {
		var err error
		normalized, err = worktree.NormalizePathFilters(a.dir, filters)
		if err != nil {
			return nil, fmt.Errorf("failed to normalize filters: %w", err)
		}
	}

	var oldIdx core.Index
	if err := a.store.LoadIndex(&oldIdx); err != nil {
		return nil, err
	}

	if !force {
		hasPending, err := a.hasPendingStagedChanges(&oldIdx, normalized)
		if err != nil {
			return nil, fmt.Errorf("failed to check pending staged changes: %w", err)
		}
		if hasPending {
			return nil, fmt.Errorf("unsaved changes exist (use --force to discard)")
		}
		currentCommit, err := a.currentCommit()
		if err != nil {
			return nil, fmt.Errorf("failed to load current commit: %w", err)
		}
		dirty, err := a.wt.HasModifications(currentCommit, &oldIdx, normalized)
		if err != nil {
			return nil, fmt.Errorf("failed to check worktree modifications: %w", err)
		}
		if dirty {
			return nil, fmt.Errorf("working tree has unsaved modifications (use --force to discard)")
		}
	}

	commit, err := a.ResolveCommit(version)
	if err != nil {
		return nil, err
	}

	targetTree, err := a.store.GetTree(commit.TreeHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load target tree: %w", err)
	}

	reader := core.NewTreeReader(a.store)
	targetBlobs, err := reader.ListBlobs(targetTree, "")
	if err != nil {
		return nil, err
	}

	if hasFilter {
		for i, filter := range normalized {
			matched := false
			for _, b := range targetBlobs {
				if worktree.PathMatchesAny(b.Path, []string{filter}) {
					matched = true
					break
				}
			}
			if !matched {
				return nil, fmt.Errorf("'%s' did not match any files in version %s", filters[i], version)
			}
		}
		targetBlobs = worktree.FilterBlobs(targetBlobs, normalized)
	}

	targetPaths := make(map[string]bool)
	for _, b := range targetBlobs {
		targetPaths[b.Path] = true
	}

	prevBlobs := make(map[string]string)
	currentBranch := a.CurrentBranch()
	if currentHash, err := a.store.GetRef(currentBranch); err == nil {
		if currentHash != commit.Hash && currentHash != "" {
			if currentCommit, err := a.findCommitByHash(currentHash); err == nil {
				if t, err := a.store.GetTree(currentCommit.TreeHash); err == nil {
					if prevBlobsList, err := reader.ListBlobs(t, ""); err == nil {
						for _, b := range prevBlobsList {
							prevBlobs[b.Path] = b.Hash
						}
					}
				}
			}
		}
	}

	newIdx := &core.Index{}
	if hasFilter {
		for _, e := range oldIdx.Entries {
			if !worktree.PathMatchesAny(e.Path, normalized) {
				newIdx.Add(e)
			}
		}
	}

	var deletedPaths []string

	for _, b := range targetBlobs {
		entry, err := a.wt.WriteBlob(b)
		if err != nil {
			return nil, err
		}
		newIdx.Add(entry)
	}

	var added, modified int
	for _, b := range targetBlobs {
		prevHash, inPrev := prevBlobs[b.Path]
		if !inPrev {
			added++
		} else if prevHash != b.Hash {
			modified++
		}
	}

	var deleted int
	for path := range prevBlobs {
		if targetPaths[path] {
			continue
		}
		if hasFilter && !worktree.PathMatchesAny(path, normalized) {
			continue
		}
		if err := core.ValidateTreePath(path); err != nil {
			continue
		}
		fullPath := filepath.Join(a.dir, filepath.FromSlash(path))
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
		if hasFilter && !worktree.PathMatchesAny(entry.Path, normalized) {
			continue
		}
		if err := core.ValidateTreePath(entry.Path); err != nil {
			continue
		}
		fullPath := filepath.Join(a.dir, filepath.FromSlash(entry.Path))
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		deleted++
		deletedPaths = append(deletedPaths, entry.Path)
	}

	a.wt.CleanEmptyDirs(deletedPaths)

	if err := a.store.SaveIndex(newIdx); err != nil {
		return nil, fmt.Errorf("failed to update index: %w", err)
	}

	oldIdxSnapshot := make([]core.IndexEntry, len(oldIdx.Entries))
	copy(oldIdxSnapshot, oldIdx.Entries)
	if err := a.recordOperationWithIndex(OpRestore, fmt.Sprintf("restore %s", version), []RefChange{}, oldIdxSnapshot); err != nil {
		return nil, err
	}

	return &RestoreResult{
		Version:  version,
		Added:    added,
		Modified: modified,
		Deleted:  deleted,
	}, nil
}
