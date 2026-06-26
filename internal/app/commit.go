package app

import (
	"fmt"
	"os"
	"sort"

	"github.com/drift/drift/internal/core"
)

type SaveOptions struct {
	Amend bool
	All   bool
	Tag   string
}

type SaveResult struct {
	ID          string
	Message     string
	Branch      string
	ChangedPaths []string
	Amended     bool
}

func (a *App) Save(msg string, opts SaveOptions) (*SaveResult, error) {
	if a.config == nil {
		return nil, fmt.Errorf("repository config is not initialized")
	}

	if opts.All {
		var idx core.Index
		if err := a.store.LoadIndex(&idx); err != nil {
			return nil, fmt.Errorf("failed to load index: %w", err)
		}
		if _, _, err := a.wt.StageAll(&idx); err != nil {
			return nil, fmt.Errorf("failed to stage changes: %w", err)
		}
		if err := a.store.SaveIndex(&idx); err != nil {
			return nil, fmt.Errorf("failed to save index: %w", err)
		}
	}

	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}

	if len(idx.Entries) == 0 {
		return nil, fmt.Errorf("nothing to save (use 'drift add' first, or 'drift save --all')")
	}

	if opts.Tag != "" {
		if err := validateTagLabel(opts.Tag); err != nil {
			return nil, fmt.Errorf("failed to tag version: %w", err)
		}
	}

	builder := core.NewTreeBuilder(func(t *core.Tree) error {
		return a.store.PutTree(t)
	})

	tree, err := builder.BuildFromIndex(&idx)
	if err != nil {
		return nil, fmt.Errorf("failed to build tree: %w", err)
	}

	branch := a.CurrentBranch()
	if _, err := a.store.GetRef("HEAD"); err != nil {
		if err := a.store.SaveRef("HEAD", branch); err != nil {
			return nil, fmt.Errorf("failed to initialize HEAD: %w", err)
		}
	}

	branchCommits, err := a.store.ListBranchCommits(branch)
	if err != nil {
		return nil, fmt.Errorf("failed to list branch commits: %w", err)
	}
	branchCommitCount := len(branchCommits)

	parentHash := ""
	if branchCommitCount > 0 {
		parentHash = branchCommits[0].Hash
		if branchCommits[0].TreeHash == tree.Hash {
			if opts.Amend {
				return nil, fmt.Errorf("nothing to amend (stage changes first with 'drift add')")
			}
			return nil, fmt.Errorf("nothing changed since last version (use 'drift add' after modifying files)")
		}
	}

	// Check tag uniqueness BEFORE any persistence so that a tag conflict
	// does not leave a half-written commit + branch ref behind.
	if opts.Tag != "" {
		tagRef := "tags/" + opts.Tag
		existing, err := a.store.GetRef(tagRef)
		if err == nil && existing != "" {
			// Amend: allow overwriting when the tag already points to the
			// commit being amended (it will be moved forward). A tag
			// pointing elsewhere is a real conflict.
			if !opts.Amend || branchCommitCount == 0 || existing != branchCommits[0].Hash {
				return nil, fmt.Errorf("tag %q already exists", opts.Tag)
			}
		}
	}

	stagedPaths := a.computeChangedPaths(&idx, branchCommits)
	author := a.Author()

	if opts.Amend {
		if branchCommitCount == 0 {
			return nil, fmt.Errorf("no version to amend (create one first with 'drift save')")
		}
		lastCommit := branchCommits[0]
		message := msg
		if message == "" {
			message = lastCommit.Message
		}
		parentHash = lastCommit.Parent

		commit := core.NewCommit(message, parentHash, branch, tree.Hash, author)

		prevBranchHash := lastCommit.Hash
		if err := a.store.SaveCommitTransaction(commit, branch, &idx); err != nil {
			return nil, fmt.Errorf("failed to save amended commit: %w", err)
		}

		changes := []RefChange{
			{Ref: branch, Before: prevBranchHash, After: commit.Hash},
		}

		// Update existing tags that point to the old commit so they
		// follow the amended commit.
		oldTags := a.TagsByHash()
		if labels, ok := oldTags[prevBranchHash]; ok {
			for _, label := range labels {
				tagRef := "tags/" + label
				if err := a.store.SaveRef(tagRef, commit.Hash); err != nil {
					return nil, fmt.Errorf("failed to update tag %q: %w", label, err)
				}
				changes = append(changes, RefChange{Ref: tagRef, Before: prevBranchHash, After: commit.Hash})
			}
		}

		if opts.Tag != "" {
			tagRef := "tags/" + opts.Tag
			if err := a.store.SaveRef(tagRef, commit.Hash); err != nil {
				return nil, fmt.Errorf("failed to save tag: %w", err)
			}
			changes = append(changes, RefChange{Ref: tagRef, Before: "", After: commit.Hash})
		}
		if err := a.recordOperation(OpSave, fmt.Sprintf("amend %s on %s", commit.ID, branch), changes); err != nil {
			return nil, err
		}

		// AutoSync after amend (best-effort, non-fatal).
		if err := a.AutoSync(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: sync failed: %v\n", err)
		}

		return &SaveResult{
			ID:          commit.ID,
			Message:     message,
			Branch:      branch,
			ChangedPaths: stagedPaths,
			Amended:      true,
		}, nil
	}

	commit := core.NewCommit(msg, parentHash, branch, tree.Hash, author)

	prevBranchHash := ""
	if branchCommitCount > 0 {
		prevBranchHash = branchCommits[0].Hash
	}
	if err := a.store.SaveCommitTransaction(commit, branch, &idx); err != nil {
		return nil, fmt.Errorf("failed to save commit: %w", err)
	}

	desc := fmt.Sprintf("save %s on %s", commit.ID, branch)
	if msg != "" {
		desc = fmt.Sprintf("save %s (%s) on %s", commit.ID, msg, branch)
	}
	changes := []RefChange{
		{Ref: branch, Before: prevBranchHash, After: commit.Hash},
	}
	if opts.Tag != "" {
		tagRef := "tags/" + opts.Tag
		if err := a.store.SaveRef(tagRef, commit.Hash); err != nil {
			return nil, fmt.Errorf("failed to save tag: %w", err)
		}
		changes = append(changes, RefChange{Ref: tagRef, Before: "", After: commit.Hash})
	}
	if err := a.recordOperation(OpSave, desc, changes); err != nil {
		return nil, err
	}

	// AutoSync after save (best-effort, non-fatal).
	if err := a.AutoSync(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: sync failed: %v\n", err)
	}

	return &SaveResult{
		ID:          commit.ID,
		Message:     msg,
		Branch:      branch,
		ChangedPaths: stagedPaths,
	}, nil
}

func (a *App) computeChangedPaths(idx *core.Index, branchCommits []*core.Commit) []string {
	if len(branchCommits) == 0 {
		paths := make([]string, len(idx.Entries))
		for i, e := range idx.Entries {
			paths[i] = e.Path
		}
		return paths
	}

	parent := branchCommits[0]
	if parent.TreeHash == "" {
		paths := make([]string, len(idx.Entries))
		for i, e := range idx.Entries {
			paths[i] = e.Path
		}
		return paths
	}

	tree, err := a.store.GetTree(parent.TreeHash)
	if err != nil {
		paths := make([]string, len(idx.Entries))
		for i, e := range idx.Entries {
			paths[i] = e.Path
		}
		return paths
	}

	reader := core.NewTreeReader(a.store)
	parentBlobs, err := reader.ListBlobs(tree, "")
	if err != nil {
		paths := make([]string, len(idx.Entries))
		for i, e := range idx.Entries {
			paths[i] = e.Path
		}
		return paths
	}

	parentFiles := make(map[string]string, len(parentBlobs))
	for _, b := range parentBlobs {
		parentFiles[b.Path] = b.Hash
	}

	changedSet := make(map[string]bool)

	for _, e := range idx.Entries {
		parentHash, inParent := parentFiles[e.Path]
		if !inParent || parentHash != e.Hash {
			changedSet[e.Path] = true
		}
	}

	for path := range parentFiles {
		if !idx.Has(path) {
			changedSet[path] = true
		}
	}

	changed := make([]string, 0, len(changedSet))
	for path := range changedSet {
		changed = append(changed, path)
	}
	sort.Strings(changed)
	return changed
}
