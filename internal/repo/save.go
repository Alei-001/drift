package repo

import (
	"fmt"
	"strconv"

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

	stagedPaths := make([]string, len(idx.Entries))
	for i, e := range idx.Entries {
		stagedPaths[i] = e.Path
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

	author := core.Signature{
		Name:  r.Config.User.Name,
		Email: r.Config.User.Email,
	}

	if opts.Amend {
		if branchCommitCount == 0 {
			return nil, fmt.Errorf("no version to amend (create one first with 'drift save')")
		}
		lastCommit := branchCommits[0]
		id := lastCommit.ID
		message := opts.Message
		if message == "" {
			message = lastCommit.Message
		}
		parentHash = lastCommit.Parent

		commit := core.NewCommit(id, message, parentHash, branch, tree.Hash, author)

		prevBranchHash := lastCommit.Hash
		if err := r.Store.SaveCommitTransaction(commit, branch, &idx); err != nil {
			return nil, fmt.Errorf("failed to save amended commit: %w", err)
		}

		r.RecordOperation(OpSave, fmt.Sprintf("amend %s on %s", id, branch), []RefChange{
			{Ref: branch, Before: prevBranchHash, After: commit.Hash},
		})

		if opts.Name != "" {
			if err := r.AddName(id, opts.Name); err != nil {
				// Non-fatal: name assignment failure shouldn't block the save.
			}
		}

		return &SaveResult{
			ID:          id,
			Message:     message,
			Branch:      branch,
			StagedPaths: stagedPaths,
			Amended:     true,
		}, nil
	}

	id := "v" + strconv.Itoa(branchCommitCount+1)
	commit := core.NewCommit(id, opts.Message, parentHash, branch, tree.Hash, author)

	prevBranchHash := ""
	if branchCommitCount > 0 {
		prevBranchHash = branchCommits[0].Hash
	}
	if err := r.Store.SaveCommitTransaction(commit, branch, &idx); err != nil {
		return nil, fmt.Errorf("failed to save commit: %w", err)
	}

	desc := fmt.Sprintf("save %s on %s", id, branch)
	if opts.Message != "" {
		desc = fmt.Sprintf("save %s (%s) on %s", id, opts.Message, branch)
	}
	r.RecordOperation(OpSave, desc, []RefChange{
		{Ref: branch, Before: prevBranchHash, After: commit.Hash},
	})

	if opts.Name != "" {
		if err := r.AddName(id, opts.Name); err != nil {
			// Non-fatal.
		}
	}

	return &SaveResult{
		ID:          id,
		Message:     opts.Message,
		Branch:      branch,
		StagedPaths: stagedPaths,
	}, nil
}
