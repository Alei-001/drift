package app

import (
	"fmt"
	"os"
	"path/filepath"
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

// parentBlobsByPath loads the current branch's latest commit tree as a
// path -> BlobEntry map. Returns an empty map when no commit exists yet.
func (a *App) parentBlobsByPath() (map[string]core.BlobEntry, error) {
	blobs, err := a.wt.LoadParentTreeBlobs()
	if err != nil {
		return nil, err
	}
	m := make(map[string]core.BlobEntry, len(blobs))
	for _, b := range blobs {
		m[b.Path] = b
	}
	return m, nil
}

// resetIndexEntryToCommit restores an index entry for path to its committed
// state: the entry uses the committed blob's hash and mode, plus on-disk
// mtime/size when the working file is present (keeping the status fast-path
// effective). Used by Unstage to undo staging without breaking the
// "index is a full snapshot of tracked files" invariant.
func (a *App) resetIndexEntryToCommit(idx *core.Index, path string, blob core.BlobEntry) error {
	entry := core.IndexEntry{
		Path: path,
		Hash: blob.Hash,
		Mode: blob.Mode,
	}
	fullPath := filepath.Join(a.dir, filepath.FromSlash(path))
	if info, err := os.Lstat(fullPath); err == nil {
		entry.ModifiedAt = info.ModTime()
		entry.Size = info.Size()
	}
	return idx.Add(entry)
}

func (a *App) Unstage(paths []string) (unstaged []string, notFound []string, err error) {
	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		return nil, nil, fmt.Errorf("failed to load index: %w", err)
	}

	parentBlobs, err := a.parentBlobsByPath()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load parent tree: %w", err)
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

	toReset := make([]string, 0, len(matched))
	for p := range matched {
		toReset = append(toReset, p)
	}
	sort.Strings(toReset)

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

	// Reset each matched path to its committed state, preserving the
	// full-snapshot invariant:
	//   - path in parent commit -> restore the committed IndexEntry
	//   - path not in parent commit (newly staged add) -> remove from index
	//     (the file becomes untracked again)
	for _, p := range toReset {
		if blob, inParent := parentBlobs[p]; inParent {
			if err := a.resetIndexEntryToCommit(&idx, p, blob); err != nil {
				return nil, nil, fmt.Errorf("failed to reset %s: %w", p, err)
			}
		} else {
			idx.Remove(p)
		}
	}

	if len(toReset) > 0 {
		if err := a.store.SaveIndex(&idx); err != nil {
			return nil, nil, fmt.Errorf("failed to save index: %w", err)
		}
	}

	return toReset, notFound, nil
}

// ClearStaging resets the staging area to a full snapshot of the current
// branch's latest commit (rather than emptying it). This keeps the "index is
// a full snapshot of tracked files" invariant, so a subsequent `drift add .`
// or `drift status` does not falsely report committed-but-unstaged files as
// deleted/untracked. In a fresh repository with no commits, the index is
// cleared to empty.
func (a *App) ClearStaging() error {
	idx, err := a.wt.BuildIndexFromCommit()
	if err != nil {
		return fmt.Errorf("failed to rebuild index from commit: %w", err)
	}
	return a.store.SaveIndex(idx)
}
