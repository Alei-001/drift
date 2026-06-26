package app

import (
	"fmt"
	"sort"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/worktree"
)

// AddResult holds the outcome of staging paths.
type AddResult struct {
	Added   []string
	Skipped []string
}

func (a *App) Add(paths []string) (*AddResult, error) {
	expanded, err := worktree.ExpandAddPaths(a.dir, paths)
	if err != nil {
		return nil, err
	}
	if len(expanded) == 0 {
		return nil, fmt.Errorf("no matching files found")
	}

	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}

	var added, skipped []string
	if len(expanded) == 1 && expanded[0] == "." {
		added, skipped, err = a.wt.StageAll(&idx)
	} else {
		added, skipped, err = a.wt.StagePaths(&idx, expanded)
	}
	if err != nil {
		return nil, err
	}

	if err := a.store.SaveIndex(&idx); err != nil {
		return nil, fmt.Errorf("failed to save index: %w", err)
	}

	return &AddResult{Added: added, Skipped: skipped}, nil
}

func (a *App) Unstage(paths []string) (unstaged []string, notFound []string, err error) {
	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		return nil, nil, fmt.Errorf("failed to load index: %w", err)
	}

	// Match index entries against user-supplied path patterns. Unlike Add,
	// Unstage does not require paths to exist in the working directory —
	// only in the index.
	matched := make(map[string]bool)
	for _, entry := range idx.Entries {
		if worktree.PathMatchesAny(entry.Path, paths) {
			matched[entry.Path] = true
		}
	}

	toRemove := make([]string, 0, len(matched))
	for p := range matched {
		toRemove = append(toRemove, p)
	}
	sort.Strings(toRemove)

	// A user-supplied path is "not found" only if it matched nothing in
	// the index. Directory paths (e.g. "sub") that matched child entries
	// (e.g. "sub/x.txt") are NOT reported as not-found.
	matchedByInput := make(map[string]bool, len(paths))
	for _, entry := range idx.Entries {
		for _, p := range paths {
			if worktree.PathMatchesAny(entry.Path, []string{p}) {
				matchedByInput[p] = true
				break
			}
		}
	}
	for _, p := range paths {
		if !matchedByInput[p] {
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
