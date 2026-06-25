package app

import (
	"fmt"
	"sort"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/worktree"
)

func (a *App) Add(paths []string) (int, error) {
	expanded, err := worktree.ExpandAddPaths(a.dir, paths)
	if err != nil {
		return 0, err
	}
	if len(expanded) == 0 {
		return 0, fmt.Errorf("no matching files found")
	}

	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		return 0, fmt.Errorf("failed to load index: %w", err)
	}

	var added int
	if len(expanded) == 1 && expanded[0] == "." {
		added, err = a.wt.StageAll(&idx)
	} else {
		added, err = a.wt.StagePaths(&idx, expanded)
	}
	if err != nil {
		return 0, err
	}

	if err := a.store.SaveIndex(&idx); err != nil {
		return 0, fmt.Errorf("failed to save index: %w", err)
	}

	return added, nil
}

func (a *App) Unstage(paths []string) (unstaged []string, notFound []string, err error) {
	expanded, err := worktree.ExpandAddPaths(a.dir, paths)
	if err != nil {
		return nil, nil, err
	}

	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		return nil, nil, fmt.Errorf("failed to load index: %w", err)
	}

	matched := make(map[string]bool)
	for _, entry := range idx.Entries {
		if worktree.PathMatchesAny(entry.Path, expanded) {
			matched[entry.Path] = true
		}
	}

	toRemove := make([]string, 0, len(matched))
	for p := range matched {
		toRemove = append(toRemove, p)
	}
	sort.Strings(toRemove)

	// Find paths that were requested but not found in index
	for _, p := range expanded {
		if !matched[p] {
			notFound = append(notFound, p)
		}
	}

	for _, p := range toRemove {
		idx.Remove(p)
	}

	if len(toRemove) > 0 {
		if err := a.store.SaveIndex(&idx); err != nil {
			return nil, nil, fmt.Errorf("failed to save index: %w", err)
		}
	}

	return toRemove, notFound, nil
}

func (a *App) ClearStaging() error {
	return a.store.SaveIndex(&core.Index{})
}
