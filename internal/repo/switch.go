package repo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/worktree"
)

// SwitchOptions controls the behavior of Repository.Switch.
type SwitchOptions struct {
	Force  bool
	Create bool
}

// SwitchResult contains the outcome of a switch operation.
type SwitchResult struct {
	Branch      string
	Created     bool
	WIPSaved    bool
	EmptyBranch bool
}

// Switch changes the current branch and reconciles the working tree.
func (r *Repository) Switch(branch string, opts SwitchOptions) (*SwitchResult, error) {
	currentBranch := r.CurrentBranch()
	if branch == currentBranch {
		return &SwitchResult{Branch: branch}, nil
	}

	result := &SwitchResult{Branch: branch}

	var idx core.Index
	if err := r.Store.LoadIndex(&idx); err != nil {
		return nil, err
	}

	if !opts.Force {
		// Auto-save pending work to WIP before switching.
		if hasPending, err := r.HasPendingStagedChanges(&idx, nil); err == nil && hasPending {
			if err := r.WT.SaveWIP(currentBranch, &idx); err != nil {
				return nil, fmt.Errorf("failed to save work-in-progress: %w", err)
			}
			result.WIPSaved = true
			emptyIdx := &core.Index{}
			if err := r.Store.SaveIndex(emptyIdx); err != nil {
				return nil, fmt.Errorf("failed to clear index: %w", err)
			}
		}

		currentCommit, _ := r.CurrentCommit()
		if dirty, err := r.WT.HasModifications(currentCommit, &idx, nil); err == nil && dirty {
			if err := r.WT.StageWorktreeChanges(&idx); err != nil {
				return nil, fmt.Errorf("failed to capture worktree changes: %w", err)
			}
			if err := r.WT.SaveWIP(currentBranch, &idx); err != nil {
				return nil, fmt.Errorf("failed to save work-in-progress: %w", err)
			}
			result.WIPSaved = true
			emptyIdx := &core.Index{}
			if err := r.Store.SaveIndex(emptyIdx); err != nil {
				return nil, fmt.Errorf("failed to clear index: %w", err)
			}
		}
	}

	commitHash, err := r.Store.GetRef(branch)
	if err != nil {
		if !opts.Create {
			return nil, fmt.Errorf("branch not found: %s", branch)
		}
		parentHash, _ := r.Store.GetRef(currentBranch)
		if err := r.Store.SaveRef(branch, parentHash); err != nil {
			return nil, fmt.Errorf("failed to create branch: %w", err)
		}
		result.Created = true
		commitHash = parentHash
	} else if opts.Create {
		return nil, fmt.Errorf("branch %q already exists", branch)
	}

	reader := core.NewTreeReader(r.Store)

	currentBlobs := make(map[string]bool)
	if currentCommit, _ := r.CurrentCommit(); currentCommit != nil {
		if t, err := r.Store.GetTree(currentCommit.TreeHash); err == nil {
			if blobs, err := reader.ListBlobs(t, ""); err == nil {
				for _, b := range blobs {
					currentBlobs[b.Path] = true
				}
			}
		}
	}

	if err := r.Store.SaveRef("HEAD", branch); err != nil {
		return nil, fmt.Errorf("failed to update HEAD: %w", err)
	}

	r.RecordOperation(OpSwitch, fmt.Sprintf("switch to %s", branch), []RefChange{
		{Ref: "HEAD", Before: currentBranch, After: branch},
	})

	// Handle empty branch.
	if commitHash == "" {
		var deletedPaths []string
		for path := range currentBlobs {
			if err := core.ValidateTreePath(path); err != nil {
				continue
			}
			fullPath := filepath.Join(r.Dir, filepath.FromSlash(path))
			if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			deletedPaths = append(deletedPaths, path)
		}
		r.WT.CleanEmptyDirs(deletedPaths)
		if err := r.Store.SaveIndex(&core.Index{}); err != nil {
			return nil, fmt.Errorf("failed to update index: %w", err)
		}
		result.EmptyBranch = true
		return result, nil
	}

	commit, err := r.FindCommitByHash(commitHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load commit: %w", err)
	}

	targetTree, err := r.Store.GetTree(commit.TreeHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load tree: %w", err)
	}

	targetBlobs, err := reader.ListBlobs(targetTree, "")
	if err != nil {
		return nil, err
	}

	targetPaths := make(map[string]bool)
	for _, b := range targetBlobs {
		targetPaths[b.Path] = true
	}

	newIdx := &core.Index{}
	for _, b := range targetBlobs {
		entry, err := r.WT.WriteBlob(b)
		if err != nil {
			return nil, err
		}
		newIdx.Add(entry)
	}

	var deletedPaths []string
	for path := range currentBlobs {
		if !targetPaths[path] {
			if err := core.ValidateTreePath(path); err != nil {
				continue
			}
			fullPath := filepath.Join(r.Dir, filepath.FromSlash(path))
			if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			deletedPaths = append(deletedPaths, path)
		}
	}

	r.WT.CleanEmptyDirs(deletedPaths)

	if err := r.Store.SaveIndex(newIdx); err != nil {
		return nil, fmt.Errorf("failed to update index: %w", err)
	}

	return result, nil
}

// RestoreWIP restores work-in-progress for the given branch.
func (r *Repository) RestoreWIP(branch string) (int, error) {
	wip, err := worktree.LoadWIP(r.Store, branch)
	if err != nil {
		return 0, err
	}
	if wip == nil || len(wip.Entries) == 0 {
		return 0, nil
	}

	var idx core.Index
	if err := r.Store.LoadIndex(&idx); err != nil {
		return 0, err
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
		blob := core.BlobEntry{Path: e.Path, Hash: e.Hash, Mode: e.Mode}
		if _, err := r.WT.WriteBlob(blob); err != nil {
			continue
		}
		restored++
	}

	if err := r.Store.SaveIndex(&idx); err != nil {
		return 0, err
	}

	_ = worktree.DeleteWIP(r.Store, branch)
	return restored, nil
}
