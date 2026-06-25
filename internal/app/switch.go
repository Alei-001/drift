package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/worktree"
)

type SwitchOptions struct {
	Force  bool
	Create bool
}

type SwitchResult struct {
	Branch      string
	Created     bool
	WIPSaved    bool
	EmptyBranch bool
}

func (a *App) Switch(branch string, opts SwitchOptions) (*SwitchResult, error) {
	currentBranch := a.CurrentBranch()
	if branch == currentBranch {
		return &SwitchResult{Branch: branch}, nil
	}

	result := &SwitchResult{Branch: branch}

	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		return nil, err
	}

	if !opts.Force {
		hasPending, err := a.hasPendingStagedChanges(&idx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to check pending staged changes: %w", err)
		}

		currentCommit, err := a.currentCommit()
		if err != nil {
			return nil, fmt.Errorf("failed to load current commit: %w", err)
		}

		dirty, err := a.wt.HasModifications(currentCommit, &idx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to check worktree modifications: %w", err)
		}

		if hasPending || dirty {
			if dirty {
				if err := a.wt.StageWorktreeChanges(&idx); err != nil {
					return nil, fmt.Errorf("failed to capture worktree changes: %w", err)
				}
			}
			if err := a.wt.SaveWIP(currentBranch, &idx); err != nil {
				return nil, fmt.Errorf("failed to save work-in-progress: %w", err)
			}
			result.WIPSaved = true
			emptyIdx := &core.Index{}
			if err := a.store.SaveIndex(emptyIdx); err != nil {
				return nil, fmt.Errorf("failed to clear index: %w", err)
			}
		}
	}

	commitHash, err := a.store.GetRef(branch)
	if err != nil {
		if !opts.Create {
			return nil, fmt.Errorf("branch not found: %s", branch)
		}
		// Best-effort: parent hash for new branch creation.
		parentHash, _ := a.store.GetRef(currentBranch)
		if err := a.store.SaveRef(branch, parentHash); err != nil {
			return nil, fmt.Errorf("failed to create branch: %w", err)
		}
		result.Created = true
		commitHash = parentHash
	} else if opts.Create {
		return nil, fmt.Errorf("branch %q already exists", branch)
	}

	// Read current commit's blob set for cleanup. Non-fatal: if the read
	// fails, stale files may remain on disk but the switch still succeeds.
	reader := core.NewTreeReader(a.store)
	currentBlobs := make(map[string]bool)
	if cc, err := a.currentCommit(); err == nil && cc != nil {
		if t, err := a.store.GetTree(cc.TreeHash); err == nil {
			if blobs, err := reader.ListBlobs(t, ""); err == nil {
				for _, b := range blobs {
					currentBlobs[b.Path] = true
				}
			}
		}
	}

	if commitHash == "" {
		var deletedPaths []string
		for path := range currentBlobs {
			if err := core.ValidateTreePath(path); err != nil {
				continue
			}
			fullPath := filepath.Join(a.dir, filepath.FromSlash(path))
			if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			deletedPaths = append(deletedPaths, path)
		}
		a.wt.CleanEmptyDirs(deletedPaths)

		// HEAD and index update at the end, after all worktree ops.
		if err := a.store.SaveRef("HEAD", branch); err != nil {
			return nil, fmt.Errorf("failed to update HEAD: %w", err)
		}
		if err := a.recordOperation(OpSwitch, fmt.Sprintf("switch to %s", branch), []RefChange{
			{Ref: "HEAD", Before: currentBranch, After: branch},
		}); err != nil {
			return nil, err
		}
		if err := a.store.SaveIndex(&core.Index{}); err != nil {
			return nil, fmt.Errorf("failed to update index: %w", err)
		}
		result.EmptyBranch = true
		return result, nil
	}

	commit, err := a.findCommitByHash(commitHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load commit: %w", err)
	}

	targetTree, err := a.store.GetTree(commit.TreeHash)
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
		entry, err := a.wt.WriteBlob(b)
		if err != nil {
			return nil, err
		}
		if err := newIdx.Add(entry); err != nil {
			return nil, fmt.Errorf("failed to add %s to index: %w", entry.Path, err)
		}
	}

	var deletedPaths []string
	for path := range currentBlobs {
		if !targetPaths[path] {
			if err := core.ValidateTreePath(path); err != nil {
				continue
			}
			fullPath := filepath.Join(a.dir, filepath.FromSlash(path))
			if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			deletedPaths = append(deletedPaths, path)
		}
	}

	a.wt.CleanEmptyDirs(deletedPaths)

	if err := a.store.SaveIndex(newIdx); err != nil {
		return nil, fmt.Errorf("failed to update index: %w", err)
	}

	// HEAD and operation log at the very end, after all worktree ops.
	if err := a.store.SaveRef("HEAD", branch); err != nil {
		return nil, fmt.Errorf("failed to update HEAD: %w", err)
	}
	if err := a.recordOperation(OpSwitch, fmt.Sprintf("switch to %s", branch), []RefChange{
		{Ref: "HEAD", Before: currentBranch, After: branch},
	}); err != nil {
		return nil, err
	}

	return result, nil
}

func (a *App) RestoreWIP(branch string) (int, error) {
	wip, err := worktree.LoadWIP(a.store, branch)
	if err != nil {
		return 0, err
	}
	if wip == nil || len(wip.Entries) == 0 {
		return 0, nil
	}

	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		return 0, err
	}

	var restored int
	var errs []error
	for _, e := range wip.Entries {
		entry := core.IndexEntry{
			Path: e.Path,
			Hash: e.Hash,
			Mode: e.Mode,
		}
		if err := idx.Add(entry); err != nil {
			errs = append(errs, fmt.Errorf("failed to add %s to index: %w", e.Path, err))
			continue
		}
		blob := core.BlobEntry{Path: e.Path, Hash: e.Hash, Mode: e.Mode}
		if _, err := a.wt.WriteBlob(blob); err != nil {
			errs = append(errs, fmt.Errorf("failed to write %s: %w", e.Path, err))
			continue
		}
		restored++
	}

	if err := a.store.SaveIndex(&idx); err != nil {
		return 0, err
	}

	// Non-fatal: WIP cleanup failure may leave stale data but doesn't
	// affect the restored working tree.
	_ = worktree.DeleteWIP(a.store, branch)
	return restored, errors.Join(errs...)
}
