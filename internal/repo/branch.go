package repo

import (
	"fmt"
	"sort"
)

// ListBranches returns all branch names (excluding HEAD), sorted alphabetically.
func (r *Repository) ListBranches() ([]string, error) {
	refs, err := r.Store.ListRefs()
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	var names []string
	for name := range refs {
		if name == "HEAD" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// CreateBranch creates a new branch pointing to the current commit.
func (r *Repository) CreateBranch(name string) error {
	if existing, err := r.Store.GetRef(name); err == nil || existing != "" {
		_ = existing
		return fmt.Errorf("branch %q already exists", name)
	}

	currentBranch := r.CurrentBranch()
	commitHash, err := r.Store.GetRef(currentBranch)
	if err != nil {
		commitHash = ""
	}

	if err := r.Store.SaveRef(name, commitHash); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}
	return nil
}

// DeleteBranch removes a branch ref. It refuses to delete the current branch or HEAD.
func (r *Repository) DeleteBranch(name string) error {
	if name == "HEAD" {
		return fmt.Errorf("cannot delete HEAD")
	}

	currentBranch := r.CurrentBranch()
	if name == currentBranch {
		return fmt.Errorf("cannot delete the currently checked-out branch %q (switch to another branch first)", name)
	}

	branchHash, _ := r.Store.GetRef(name)

	if err := r.Store.DeleteRef(name); err != nil {
		return err
	}

	r.RecordOperation(OpBranchDelete, fmt.Sprintf("delete branch %s", name), []RefChange{
		{Ref: name, Before: branchHash, After: ""},
	})
	return nil
}

// RenameBranch renames a branch. HEAD is updated if it pointed at the old name.
func (r *Repository) RenameBranch(oldName, newName string) error {
	headBefore, _ := r.Store.GetRef("HEAD")

	if err := r.Store.RenameRef(oldName, newName); err != nil {
		return err
	}

	headAfter, _ := r.Store.GetRef("HEAD")
	changes := []RefChange{
		{Ref: oldName, Before: "", After: ""},
	}
	if headBefore != headAfter {
		changes = append(changes, RefChange{Ref: "HEAD", Before: headBefore, After: headAfter})
	}
	r.RecordOperation(OpBranchRename, fmt.Sprintf("rename %s -> %s", oldName, newName), changes)
	return nil
}
