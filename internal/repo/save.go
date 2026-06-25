package repo

import (
	"fmt"
	"sort"

	"github.com/drift/drift/internal/core"
)

// SaveOptions controls the behavior of Repository.Save.
type SaveOptions struct {
	Message string
	Amend   bool
	All     bool // auto-stage all changes before saving
	Name    string // assign a version name after saving
}

// SaveResult contains the outcome of a save operation.
type SaveResult struct {
	ID          string
	Message     string
	Branch      string
	StagedPaths []string
	Amended     bool
}

// Save creates a new version from the staging area.
func (r *Repository) Save(opts SaveOptions) (*SaveResult, error) {
	if r.Config == nil {
		return nil, fmt.Errorf("repository config is not initialized")
	}

	// --all: auto-stage all changes before saving.
	if opts.All {
		var idx core.Index
		if err := r.Store.LoadIndex(&idx); err != nil {
			return nil, fmt.Errorf("failed to load index: %w", err)
		}
		if _, err := r.WT.StageAll(&idx); err != nil {
			return nil, fmt.Errorf("failed to stage changes: %w", err)
		}
		if err := r.Store.SaveIndex(&idx); err != nil {
			return nil, fmt.Errorf("failed to save index: %w", err)
		}
	}

	var idx core.Index
	if err := r.Store.LoadIndex(&idx); err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}

	if len(idx.Entries) == 0 {
		return nil, fmt.Errorf("nothing to save (use 'drift add' first, or 'drift save --all')")
	}

	builder := core.NewTreeBuilder(func(t *core.Tree) error {
		return r.Store.PutTree(t)
	})

	tree, err := builder.BuildFromIndex(&idx)
	if err != nil {
		return nil, fmt.Errorf("failed to build tree: %w", err)
	}

	branch := r.CurrentBranch()
	if _, err := r.Store.GetRef("HEAD"); err != nil {
		if err := r.Store.SaveRef("HEAD", branch); err != nil {
			return nil, fmt.Errorf("failed to initialize HEAD: %w", err)
		}
	}

	branchCommits, err := r.Store.ListBranchCommits(branch)
	if err != nil {
		return nil, fmt.Errorf("failed to list branch commits: %w", err)
	}
	branchCommitCount := len(branchCommits)

	parentHash := ""
	if branchCommitCount > 0 {
		parentHash = branchCommits[0].Hash
		if branchCommits[0].TreeHash == tree.Hash && !opts.Amend {
			return nil, fmt.Errorf("nothing changed since last version (use 'drift add' after modifying files)")
		}
	}

	// Compute only the paths that actually changed vs the parent commit,
	// not the entire index. This avoids listing all tracked files on
	// every save after the first one.
	stagedPaths := r.computeChangedPaths(&idx, branchCommits)

	author := r.Author()

	if opts.Amend {
		if branchCommitCount == 0 {
			return nil, fmt.Errorf("no version to amend (create one first with 'drift save')")
		}
		lastCommit := branchCommits[0]
		message := opts.Message
		if message == "" {
			message = lastCommit.Message
		}
		parentHash = lastCommit.Parent

		commit := core.NewCommit(message, parentHash, branch, tree.Hash, author)

		prevBranchHash := lastCommit.Hash
		if err := r.Store.SaveCommitTransaction(commit, branch, &idx); err != nil {
			return nil, fmt.Errorf("failed to save amended commit: %w", err)
		}

		r.RecordOperation(OpSave, fmt.Sprintf("amend %s on %s", commit.ID, branch), []RefChange{
			{Ref: branch, Before: prevBranchHash, After: commit.Hash},
		})

		if opts.Name != "" {
			if err := r.AddName(commit.ID, opts.Name); err != nil {
				// Non-fatal: name assignment failure shouldn't block the save.
			}
		}

		return &SaveResult{
			ID:          commit.ID,
			Message:     message,
			Branch:      branch,
			StagedPaths: stagedPaths,
			Amended:     true,
		}, nil
	}

	commit := core.NewCommit(opts.Message, parentHash, branch, tree.Hash, author)

	prevBranchHash := ""
	if branchCommitCount > 0 {
		prevBranchHash = branchCommits[0].Hash
	}
	if err := r.Store.SaveCommitTransaction(commit, branch, &idx); err != nil {
		return nil, fmt.Errorf("failed to save commit: %w", err)
	}

	desc := fmt.Sprintf("save %s on %s", commit.ID, branch)
	if opts.Message != "" {
		desc = fmt.Sprintf("save %s (%s) on %s", commit.ID, opts.Message, branch)
	}
	r.RecordOperation(OpSave, desc, []RefChange{
		{Ref: branch, Before: prevBranchHash, After: commit.Hash},
	})

	if opts.Name != "" {
		if err := r.AddName(commit.ID, opts.Name); err != nil {
			// Non-fatal.
		}
	}

	return &SaveResult{
		ID:          commit.ID,
		Message:     opts.Message,
		Branch:      branch,
		StagedPaths: stagedPaths,
	}, nil
}

// computeChangedPaths returns the list of paths that differ between the
// current index and the parent commit's tree. This includes added, modified,
// and deleted files. If there is no parent commit (first save), all index
// entries are returned.
func (r *Repository) computeChangedPaths(idx *core.Index, branchCommits []*core.Commit) []string {
	if len(branchCommits) == 0 {
		// First commit on this branch — all files are "changed".
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

	tree, err := r.Store.GetTree(parent.TreeHash)
	if err != nil {
		// Fallback: return all index entries.
		paths := make([]string, len(idx.Entries))
		for i, e := range idx.Entries {
			paths[i] = e.Path
		}
		return paths
	}

	reader := core.NewTreeReader(r.Store)
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

	// Use a set to deduplicate, then convert to sorted slice.
	changedSet := make(map[string]bool)

	// Added or modified files (in index).
	for _, e := range idx.Entries {
		parentHash, inParent := parentFiles[e.Path]
		if !inParent || parentHash != e.Hash {
			changedSet[e.Path] = true
		}
	}

	// Deleted files (in parent but not in index).
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
