package app

import (
	"fmt"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/worktree"
)

type WIPEntry struct {
	Path string
	Hash string
	Mode uint32
}

func (a *App) WIPList(branch string) ([]WIPEntry, error) {
	wip, err := worktree.LoadWIP(a.store, branch)
	if err != nil {
		return nil, err
	}
	if wip == nil {
		return nil, nil
	}
	entries := make([]WIPEntry, len(wip.Entries))
	for i, e := range wip.Entries {
		entries[i] = WIPEntry{
			Path: e.Path,
			Hash: e.Hash,
			Mode: e.Mode,
		}
	}
	return entries, nil
}

func (a *App) WIPListAll() ([]string, error) {
	return worktree.ListWIPBranches(a.store)
}

func (a *App) WIPSave(branch string) (int, error) {
	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		return 0, err
	}

	if err := a.wt.StageWorktreeChanges(&idx); err != nil {
		return 0, err
	}

	// Check if there are any pending staged changes. If not, this is a no-op.
	hasPending, err := a.hasPendingStagedChanges(&idx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to check pending staged changes: %w", err)
	}
	if !hasPending {
		return 0, nil
	}

	count := len(idx.Entries)

	if err := a.wt.SaveWIP(branch, &idx); err != nil {
		return 0, err
	}

	emptyIdx := &core.Index{}
	if err := a.store.SaveIndex(emptyIdx); err != nil {
		return 0, err
	}

	return count, nil
}

func (a *App) WIPRestore(branch string) (int, error) {
	return a.RestoreWIP(branch)
}

func (a *App) WIPDrop(branch string) error {
	return worktree.DeleteWIP(a.store, branch)
}
