package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/drift/drift/internal/core"
)

type HistoryOptions struct {
	Branch string
	All    bool
	Limit  int
}

func (a *App) History(opts HistoryOptions) ([]*core.Commit, error) {
	if opts.All {
		return a.allBranchHistory(opts.Limit)
	}

	branch := opts.Branch
	if branch == "" {
		branch = a.CurrentBranch()
	}

	if _, err := a.store.GetRef(branch); err != nil {
		return nil, fmt.Errorf("branch not found: %s", branch)
	}

	commits, err := a.store.ListBranchCommits(branch)
	if err != nil {
		return nil, fmt.Errorf("failed to read branch history: %w", err)
	}

	if opts.Limit > 0 && opts.Limit < len(commits) {
		commits = commits[:opts.Limit]
	}

	return commits, nil
}

func (a *App) allBranchHistory(limit int) ([]*core.Commit, error) {
	refs, err := a.store.ListRefs()
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var all []*core.Commit

	branches := make([]string, 0, len(refs))
	for name := range refs {
		branches = append(branches, name)
	}
	sort.Strings(branches)

	for _, name := range branches {
		if name == "HEAD" || strings.HasPrefix(name, "names/") {
			continue
		}
		commits, err := a.store.ListBranchCommits(name)
		if err != nil {
			continue
		}
		for _, c := range commits {
			if seen[c.Hash] {
				continue
			}
			seen[c.Hash] = true
			all = append(all, c)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp.UnixMilli() > all[j].Timestamp.UnixMilli()
	})

	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}

	return all, nil
}

func (a *App) Log(limit int) ([]OperationEntry, error) {
	entries, err := a.ReadOperations()
	if err != nil {
		return nil, err
	}

	if limit > 0 && limit < len(entries) {
		entries = entries[:limit]
	}

	return entries, nil
}

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
