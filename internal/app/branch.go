package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	"github.com/drift/drift/internal/worktree"
)

func (a *App) CurrentBranch() string {
	branch, err := a.store.GetRef("HEAD")
	if err != nil || branch == "" {
		return "main"
	}
	return branch
}

func (a *App) BranchList() ([]string, error) {
	refs, err := a.store.ListRefs()
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	type branchInfo struct {
		name      string
		timestamp int64
	}

	var branches []branchInfo
	for name := range refs {
		if name == "HEAD" {
			continue
		}
		if strings.HasPrefix(name, "tags/") {
			continue
		}
		var ts int64
		if hash, err := a.store.GetRef(name); err == nil && hash != "" {
			if c, err := a.store.GetCommit(hash); err == nil {
				ts = c.Timestamp.UnixMilli()
			}
		}
		branches = append(branches, branchInfo{name: name, timestamp: ts})
	}

	sort.Slice(branches, func(i, j int) bool {
		if branches[i].timestamp == branches[j].timestamp {
			return branches[i].name < branches[j].name
		}
		return branches[i].timestamp > branches[j].timestamp
	})

	names := make([]string, len(branches))
	for i, b := range branches {
		names[i] = b.name
	}
	return names, nil
}

func (a *App) BranchCreate(name string) error {
	if _, err := a.store.GetRef(name); err == nil {
		return fmt.Errorf("branch %q already exists", name)
	} else if !errors.Is(err, storage.ErrObjectNotFound) {
		return fmt.Errorf("failed to check branch %q: %w", name, err)
	}

	currentBranch := a.CurrentBranch()
	commitHash, err := a.store.GetRef(currentBranch)
	if err != nil {
		if !errors.Is(err, storage.ErrObjectNotFound) {
			return fmt.Errorf("failed to read current branch %q: %w", currentBranch, err)
		}
	}

	if err := a.store.SaveRef(name, commitHash); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	if err := a.recordOperation(OpBranchCreate, fmt.Sprintf("create branch %s", name), []RefChange{
		{Ref: name, Before: "", After: commitHash},
	}); err != nil {
		return err
	}
	return nil
}

func (a *App) BranchDelete(name string) error {
	if name == "HEAD" {
		return fmt.Errorf("cannot delete HEAD")
	}

	currentBranch := a.CurrentBranch()
	if name == currentBranch {
		return fmt.Errorf("cannot delete the currently checked-out branch %q (switch to another branch first)", name)
	}

	branchHash, _ := a.store.GetRef(name)

	if err := a.store.DeleteRef(name); err != nil {
		return err
	}

	if err := a.recordOperation(OpBranchDelete, fmt.Sprintf("delete branch %s", name), []RefChange{
		{Ref: name, Before: branchHash, After: ""},
	}); err != nil {
		return err
	}

	a.autoGC()

	return nil
}

func (a *App) BranchRename(oldName, newName string) error {
	headBefore, _ := a.store.GetRef("HEAD")

	oldHash, err := a.store.GetRef(oldName)
	if err != nil {
		return fmt.Errorf("failed to read branch %q before rename: %w", oldName, err)
	}

	if err := a.store.RenameRef(oldName, newName); err != nil {
		return err
	}

	newHash, _ := a.store.GetRef(newName)
	headAfter, _ := a.store.GetRef("HEAD")

	changes := []RefChange{
		{Ref: oldName, Before: oldHash, After: newHash},
		{Ref: newName, Before: "", After: newHash},
	}
	if headBefore != headAfter {
		changes = append(changes, RefChange{Ref: "HEAD", Before: headBefore, After: headAfter})
	}
	if err := a.recordOperation(OpBranchRename, fmt.Sprintf("rename %s -> %s", oldName, newName), changes); err != nil {
		return err
	}
	return nil
}

type SwitchOptions struct {
	Force  bool
	Create bool
}

type SwitchResult struct {
	Branch         string
	Created        bool
	WIPSaved       bool
	EmptyBranch    bool
	AlreadyOnBranch bool
}

func (a *App) Switch(branch string, opts SwitchOptions) (*SwitchResult, error) {
	currentBranch := a.CurrentBranch()
	if branch == currentBranch {
		return &SwitchResult{Branch: branch, AlreadyOnBranch: true}, nil
	}

	result := &SwitchResult{Branch: branch}

	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		return nil, err
	}

	if !opts.Force {
		hasPending, err := a.hasPendingStagedChanges(&idx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to check pending staged changes: %w", err)
		}

		currentCommit, err := a.currentCommit()
		if err != nil {
			return nil, fmt.Errorf("failed to load current commit: %w", err)
		}

		dirty, err := a.wt.HasModifications(currentCommit, &idx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to check worktree modifications: %w", err)
		}

		if hasPending || dirty {
			if dirty {
				if err := a.wt.StageWorktreeChanges(&idx); err != nil {
					return nil, fmt.Errorf("failed to capture worktree changes: %w", err)
				}
			}
			if err := a.wt.SaveWIP(currentBranch, &idx); err != nil {
				return nil, fmt.Errorf("failed to save work-in-progress: %w", err)
			}
			if err := a.store.SaveIndex(&core.Index{}); err != nil {
				_ = worktree.DeleteWIP(a.store, currentBranch)
				return nil, fmt.Errorf("failed to clear index: %w", err)
			}
			result.WIPSaved = true
		}
	}

	commitHash, err := a.store.GetRef(branch)
	if err != nil {
		if !opts.Create {
			return nil, fmt.Errorf("branch not found: %s", branch)
		}
		parentHash, err := a.store.GetRef(currentBranch)
		if err != nil && !errors.Is(err, storage.ErrObjectNotFound) {
			return nil, fmt.Errorf("failed to read current branch %q: %w", currentBranch, err)
		}
		if err := a.store.SaveRef(branch, parentHash); err != nil {
			return nil, fmt.Errorf("failed to create branch: %w", err)
		}
		result.Created = true
		commitHash = parentHash
	} else if opts.Create {
		return nil, fmt.Errorf("branch %q already exists", branch)
	}

	reader := core.NewTreeReader(a.store)

	if commitHash == "" {
		var deletedPaths []string
		walkErr := core.WalkWorkingDir(a.dir, func(path string, info os.FileInfo) error {
			if err := core.ValidateTreePath(path); err != nil {
				return nil
			}
			fullPath := filepath.Join(a.dir, filepath.FromSlash(path))
			if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
				return err
			}
			deletedPaths = append(deletedPaths, path)
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("failed to clean worktree: %w", walkErr)
		}
		a.wt.CleanEmptyDirs(deletedPaths)

		if err := a.store.SaveRef("HEAD", branch); err != nil {
			return nil, fmt.Errorf("failed to update HEAD: %w", err)
		}
		refChanges := []RefChange{
			{Ref: "HEAD", Before: currentBranch, After: branch},
		}
		if result.Created {
			refChanges = append(refChanges, RefChange{Ref: branch, Before: "", After: commitHash})
		}
		if err := a.recordOperation(OpSwitch, fmt.Sprintf("switch to %s", branch), refChanges); err != nil {
			return nil, err
		}
		if err := a.store.SaveIndex(&core.Index{}); err != nil {
			return nil, fmt.Errorf("failed to update index: %w", err)
		}
		result.EmptyBranch = true

		if wip, _ := worktree.LoadWIP(a.store, branch); wip != nil && len(wip.Entries) > 0 {
			if _, err := a.RestoreWIP(branch); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to restore WIP for %s: %v\n", branch, err)
			}
		}

		return result, nil
	}

	commit, err := a.findCommitByHash(commitHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load commit: %w", err)
	}

	targetTree, err := a.store.GetTree(commit.TreeHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load tree: %w", err)
	}

	targetBlobs, err := reader.ListBlobs(targetTree, "")
	if err != nil {
		return nil, err
	}

	targetPaths := make(map[string]bool)
	for _, b := range targetBlobs {
		targetPaths[b.Path] = true
	}

	newIdx := &core.Index{}
	for _, b := range targetBlobs {
		entry, err := a.wt.WriteBlob(b)
		if err != nil {
			return nil, err
		}
		if err := newIdx.Add(entry); err != nil {
			return nil, fmt.Errorf("failed to add %s to index: %w", entry.Path, err)
		}
	}

	var deletedPaths []string
	walkErr := core.WalkWorkingDir(a.dir, func(path string, info os.FileInfo) error {
		if targetPaths[path] {
			return nil
		}
		if err := core.ValidateTreePath(path); err != nil {
			return nil
		}
		fullPath := filepath.Join(a.dir, filepath.FromSlash(path))
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		deletedPaths = append(deletedPaths, path)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("failed to clean worktree: %w", walkErr)
	}

	a.wt.CleanEmptyDirs(deletedPaths)

	if err := a.store.SaveIndex(newIdx); err != nil {
		return nil, fmt.Errorf("failed to update index: %w", err)
	}

	if err := a.store.SaveRef("HEAD", branch); err != nil {
		return nil, fmt.Errorf("failed to update HEAD: %w", err)
	}
	refChanges := []RefChange{
		{Ref: "HEAD", Before: currentBranch, After: branch},
	}
	if result.Created {
		refChanges = append(refChanges, RefChange{Ref: branch, Before: "", After: commitHash})
	}
	if err := a.recordOperation(OpSwitch, fmt.Sprintf("switch to %s", branch), refChanges); err != nil {
		return nil, err
	}

	if wip, _ := worktree.LoadWIP(a.store, branch); wip != nil && len(wip.Entries) > 0 {
		if _, err := a.RestoreWIP(branch); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to restore WIP for %s: %v\n", branch, err)
		}
	}

	return result, nil
}

func (a *App) RestoreWIP(branch string) (int, error) {
	wip, err := worktree.LoadWIP(a.store, branch)
	if err != nil {
		return 0, err
	}
	if wip == nil || len(wip.Entries) == 0 {
		return 0, nil
	}

	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		return 0, err
	}

	var restored int
	var errs []error
	for _, e := range wip.Entries {
		blob := core.BlobEntry{Path: e.Path, Hash: e.Hash, Mode: e.Mode}
		if _, err := a.wt.WriteBlob(blob); err != nil {
			errs = append(errs, fmt.Errorf("failed to write %s: %w", e.Path, err))
			continue
		}
		entry := core.IndexEntry{
			Path: e.Path,
			Hash: e.Hash,
			Mode: e.Mode,
		}
		if err := idx.Add(entry); err != nil {
			errs = append(errs, fmt.Errorf("failed to add %s to index: %w", e.Path, err))
			continue
		}
		restored++
	}

	if err := a.store.SaveIndex(&idx); err != nil {
		return 0, err
	}

	_ = worktree.DeleteWIP(a.store, branch)
	return restored, errors.Join(errs...)
}
