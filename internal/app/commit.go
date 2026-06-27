package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
)

type SaveOptions struct {
	Tag string
}

type SaveResult struct {
	ID           string
	Message      string
	Branch       string
	ChangedPaths []string
	TagWarning   error
}

func (a *App) Save(msg string, opts SaveOptions) (*SaveResult, error) {
	if a.config == nil {
		return nil, fmt.Errorf("repository config is not initialized")
	}

	if opts.Tag != "" {
		if err := validateTagLabel(opts.Tag); err != nil {
			return nil, fmt.Errorf("invalid tag %q: %w", opts.Tag, err)
		}
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

	var parentTree *core.Tree
	var parentHash string
	if branchCommitCount > 0 {
		parentHash = branchCommits[0].Hash
		if branchCommits[0].TreeHash != "" {
			t, treeErr := a.store.GetTree(branchCommits[0].TreeHash)
			if treeErr != nil && !errors.Is(treeErr, storage.ErrObjectNotFound) {
				return nil, fmt.Errorf("failed to load parent tree: %w", treeErr)
			}
			parentTree = t
		}
	}

	idx, changedPaths, err := a.wt.BuildChangedIndex(parentTree)
	if err != nil {
		return nil, fmt.Errorf("failed to detect changes: %w", err)
	}

	if len(changedPaths) == 0 {
		return nil, fmt.Errorf("nothing changed since last version")
	}

	for _, path := range changedPaths {
		entry, entryErr := idx.Entry(path)
		if entryErr != nil {
			continue
		}
		if a.store.HasObject(entry.Hash) {
			continue
		}
		fullPath := filepath.Join(a.dir, filepath.FromSlash(path))
		if entry.Mode == core.ModeSymlink {
			target, readErr := os.Readlink(fullPath)
			if readErr != nil {
				continue
			}
			if _, putErr := a.store.PutBlob([]byte(target)); putErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to store blob for %s: %v\n", path, putErr)
			}
		} else {
			data, readErr := os.ReadFile(fullPath)
			if readErr != nil {
				continue
			}
			if !bytes.Contains(data, []byte{0}) {
				data = bytes.ReplaceAll(data, []byte{'\r', '\n'}, []byte{'\n'})
			}
			if _, putErr := a.store.PutBlob(data); putErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to store blob for %s: %v\n", path, putErr)
			}
		}
	}

	builder := core.NewTreeBuilder(func(t *core.Tree) error {
		return a.store.PutTree(t)
	})

	var tree *core.Tree
	if parentTree != nil {
		tree, err = builder.BuildFromIndexWithBase(idx, parentTree, a.store)
	} else {
		tree, err = builder.BuildFromIndex(idx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to build tree: %w", err)
	}

	author := a.Author()
	commit, err := core.NewCommit(msg, parentHash, branch, tree.Hash, author)
	if err != nil {
		return nil, fmt.Errorf("cannot create commit: %w", err)
	}

	prevBranchHash := ""
	if branchCommitCount > 0 {
		prevBranchHash = branchCommits[0].Hash
	}
	if err := a.store.SaveCommitTransaction(commit, branch, idx); err != nil {
		return nil, fmt.Errorf("failed to save commit: %w", err)
	}

	desc := fmt.Sprintf("save %s on %s", commit.ID, branch)
	if msg != "" {
		desc = fmt.Sprintf("save %s (%s) on %s", commit.ID, msg, branch)
	}
	changes := []RefChange{
		{Ref: branch, Before: prevBranchHash, After: commit.Hash},
	}
	var tagWarning error
	if opts.Tag != "" {
		tagRef := "tags/" + opts.Tag
		existing, tagErr := a.store.GetRef(tagRef)
		if tagErr == nil && existing != "" {
			tagWarning = fmt.Errorf("tag %q already exists", opts.Tag)
		} else {
			if err := a.store.SaveRef(tagRef, commit.Hash); err != nil {
				tagWarning = fmt.Errorf("failed to save tag: %w", err)
			} else {
				changes = append(changes, RefChange{Ref: tagRef, Before: "", After: commit.Hash})
			}
		}
	}
	if err := a.recordOperation(OpSave, desc, changes); err != nil {
		return nil, err
	}

	if err := a.AutoSync(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: sync failed: %v\n", err)
	}

	a.autoGC()

	return &SaveResult{
		ID:           commit.ID,
		Message:      msg,
		Branch:       branch,
		ChangedPaths: changedPaths,
		TagWarning:   tagWarning,
	}, nil
}

// computeChangedPaths returns the paths that differ between the new tree
// (built from the index) and the parent commit's tree. Uses LazyDiffTrees to
// skip unchanged subtrees — much faster than flattening both trees.
func (a *App) computeChangedPaths(newTree *core.Tree, branchCommits []*core.Commit) []string {
	// New branch: all index entries are new.
	if len(branchCommits) == 0 {
		reader := core.NewTreeReader(a.store)
		blobs, err := reader.ListBlobs(newTree, "")
		if err != nil {
			return nil
		}
		paths := make([]string, len(blobs))
		for i, b := range blobs {
			paths[i] = b.Path
		}
		return paths
	}

	parent := branchCommits[0]
	if parent.TreeHash == "" {
		reader := core.NewTreeReader(a.store)
		blobs, err := reader.ListBlobs(newTree, "")
		if err != nil {
			return nil
		}
		paths := make([]string, len(blobs))
		for i, b := range blobs {
			paths[i] = b.Path
		}
		return paths
	}

	parentTree, err := a.store.GetTree(parent.TreeHash)
	if err != nil {
		// Fall back: list new tree blobs.
		reader := core.NewTreeReader(a.store)
		blobs, _ := reader.ListBlobs(newTree, "")
		paths := make([]string, len(blobs))
		for i, b := range blobs {
			paths[i] = b.Path
		}
		return paths
	}

	reader := core.NewTreeReader(a.store)
	changes, err := reader.LazyDiffTrees(parentTree, newTree)
	if err != nil {
		// Fall back: list new tree blobs.
		blobs, _ := reader.ListBlobs(newTree, "")
		paths := make([]string, len(blobs))
		for i, b := range blobs {
			paths[i] = b.Path
		}
		return paths
	}

	changed := make([]string, 0, len(changes))
	for _, ch := range changes {
		changed = append(changed, ch.Path)
	}
	sort.Strings(changed)
	return changed
}
