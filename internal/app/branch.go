package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/drift/drift/internal/storage"
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

	var names []string
	for name := range refs {
		if name == "HEAD" {
			continue
		}
		if strings.HasPrefix(name, "names/") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
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
		commitHash = ""
	}

	if err := a.store.SaveRef(name, commitHash); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
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

	// Best-effort: hash for undo record; DeleteRef will fail if branch doesn't exist.
	branchHash, _ := a.store.GetRef(name)

	if err := a.store.DeleteRef(name); err != nil {
		return err
	}

	if err := a.recordOperation(OpBranchDelete, fmt.Sprintf("delete branch %s", name), []RefChange{
		{Ref: name, Before: branchHash, After: ""},
	}); err != nil {
		return err
	}
	return nil
}

func (a *App) BranchRename(oldName, newName string) error {
	// Best-effort: for undo record.
	headBefore, _ := a.store.GetRef("HEAD")

	oldHash, err := a.store.GetRef(oldName)
	if err != nil {
		return fmt.Errorf("failed to read branch %q before rename: %w", oldName, err)
	}

	if err := a.store.RenameRef(oldName, newName); err != nil {
		return err
	}

	// Best-effort: for undo record.
	newHash, _ := a.store.GetRef(newName)
	headAfter, _ := a.store.GetRef("HEAD")
	changes := []RefChange{
		{Ref: oldName, Before: oldHash, After: newHash},
	}
	if headBefore != headAfter {
		changes = append(changes, RefChange{Ref: "HEAD", Before: headBefore, After: headAfter})
	}
	if err := a.recordOperation(OpBranchRename, fmt.Sprintf("rename %s -> %s", oldName, newName), changes); err != nil {
		return err
	}
	return nil
}
