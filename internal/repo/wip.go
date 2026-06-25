package repo

import (
	"fmt"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/worktree"
)

// WIPSave saves current uncommitted work (staged + unstaged) as WIP for
// the current branch. If there are no pending changes, it is a no-op.
// The index is cleared after saving so the user can start fresh.
//
// This mirrors the WIP-saving logic used by Switch, but without changing
// branches — useful as a manual stash-like operation.
func (r *Repository) WIPSave() error {
	var idx core.Index
	if err := r.Store.LoadIndex(&idx); err != nil {
		return fmt.Errorf("failed to load index: %w", err)
	}

	branch := r.CurrentBranch()

	// Capture unstaged worktree changes into the index so they are
	// included in the WIP snapshot alongside already-staged changes.
	// (HasModifications alone misses modifications to files that are
	// already in the index matching the commit, so we stage first and
	// then check for pending changes.)
	if err := r.WT.StageWorktreeChanges(&idx); err != nil {
		return fmt.Errorf("failed to capture worktree changes: %w", err)
	}

	hasPending, err := r.HasPendingStagedChanges(&idx, nil)
	if err != nil {
		return fmt.Errorf("failed to check pending staged changes: %w", err)
	}
	if !hasPending {
		return nil
	}

	if err := r.WT.SaveWIP(branch, &idx); err != nil {
		return fmt.Errorf("failed to save work-in-progress: %w", err)
	}

	emptyIdx := &core.Index{}
	if err := r.Store.SaveIndex(emptyIdx); err != nil {
		return fmt.Errorf("failed to clear index: %w", err)
	}

	return nil
}

// WIPDrop deletes the WIP for the given branch. If no WIP exists, it is a no-op.
func (r *Repository) WIPDrop(branch string) error {
	return worktree.DeleteWIP(r.Store, branch)
}
