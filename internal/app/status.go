package app

import (
	"fmt"

	"github.com/drift/drift/internal/core"
)

func (a *App) Status() (*core.Status, error) {
	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}

	commit, err := a.currentCommit()
	if err != nil {
		return nil, err
	}

	var commitTree *core.Tree
	if commit != nil && commit.TreeHash != "" {
		t, err := a.store.GetTree(commit.TreeHash)
		if err != nil {
			return nil, err
		}
		commitTree = t
	}

	status, err := core.ComputeStatus(commitTree, &idx, a.dir, a.store)
	if err != nil {
		return nil, fmt.Errorf("failed to compute status: %w", err)
	}

	return &status, nil
}
