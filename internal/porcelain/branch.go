package porcelain

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/refname"
)

// CreateBranch creates a new branch pointing at the current HEAD snapshot.
func CreateBranch(ctx context.Context, store storage.Storer, cwd, name string) error {
	if err := AcquireWorkspaceLock(cwd); err != nil {
		return fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(cwd)
	return createBranchNoLock(ctx, store, name)
}

// createBranchNoLock performs the branch creation without acquiring the
// workspace lock. Callers already holding the lock (e.g. SwitchBranch) should
// use this to avoid a non-re-entrant deadlock.
func createBranchNoLock(ctx context.Context, store storage.Storer, name string) error {
	if name == "" {
		return fmt.Errorf("branch name is empty")
	}
	if err := refname.Validate("heads/" + name); err != nil {
		return fmt.Errorf("invalid branch name: %w", err)
	}

	if _, err := store.GetRef(ctx, "heads/"+name); err == nil {
		return fmt.Errorf("branch '%s' already exists: %w", name, ErrBranchAlreadyExists)
	} else if !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("check branch existence: %w", err)
	}

	headRef, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}

	// When HEAD's target is zero (a fresh project with no commits, or a
	// detached HEAD pointing at nothing), the new branch is created empty.
	// This is allowed: cmd/branch.go detects the zero target and prints
	// "no commits yet" instead of a snapshot ID, so the user is informed
	// rather than misled. Switching to such an empty branch from a detached
	// HEAD is guarded by SwitchBranch to avoid severing history.
	if headRef.Target.IsZero() {
		// new branch will be empty; caller (cmd/branch.go) surfaces this
	}

	return store.SetRef(ctx, "heads/"+name, &core.Reference{
		Type:   core.RefTypeBranch,
		Name:   "heads/" + name,
		Target: headRef.Target,
	})
}

// ListBranches returns all branch references and the name of the current
// branch (without the "heads/" prefix). If HEAD is not a symbolic reference
// the current branch name is empty.
func ListBranches(ctx context.Context, store storage.Storer) ([]*core.Reference, string, error) {
	refs, err := store.ListRefs(ctx, "heads/")
	if err != nil {
		return nil, "", fmt.Errorf("list branches: %w", err)
	}

	current := ""
	headRef, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		return nil, "", fmt.Errorf("read HEAD: %w", err)
	}
	if headRef.SymRef != "" {
		current = strings.TrimPrefix(headRef.SymRef, "heads/")
	}

	return refs, current, nil
}

// DeleteBranch removes a branch reference. It refuses to delete:
//   - "main" (the default, protected branch)
//   - the current branch (user must switch away first)
//
// Only the reference is removed; snapshots remain in storage and can be
// reclaimed later by a future prune/GC command.
func DeleteBranch(ctx context.Context, store storage.Storer, cwd, name string) error {
	if err := AcquireWorkspaceLock(cwd); err != nil {
		return fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(cwd)

	if name == "" {
		return fmt.Errorf("branch name is empty")
	}
	if err := refname.Validate("heads/" + name); err != nil {
		return fmt.Errorf("invalid branch name: %w", err)
	}
	if name == "main" {
		return ErrCannotDeleteMain
	}

	if _, err := store.GetRef(ctx, "heads/"+name); err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("check branch existence: %w", err)
		}
		return fmt.Errorf("branch '%s' not found: %w", name, ErrBranchNotFound)
	}

	headRef, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}
	if headRef.SymRef == "heads/"+name {
		return ErrCannotDeleteCurrentBranch
	}

	return store.DeleteRef(ctx, "heads/"+name)
}

// RenameBranch renames a branch from oldName to newName. It refuses to rename:
//   - "main" (the default, protected branch)
//   - a non-existent branch
//   - to a name that already exists
//
// If the renamed branch is the current branch (HEAD points to it), HEAD is
// updated to point to the new name. Only references are modified; snapshots
// remain untouched.
//
// The operation is ordered as SetRef(new) then DeleteRef(old) so that a crash
// leaves a duplicate rather than a missing branch, which is safer to recover.
func RenameBranch(ctx context.Context, store storage.Storer, cwd, oldName, newName string) error {
	if err := AcquireWorkspaceLock(cwd); err != nil {
		return fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(cwd)

	if oldName == "" {
		return fmt.Errorf("old branch name is empty")
	}
	if newName == "" {
		return fmt.Errorf("new branch name is empty")
	}
	if oldName == "main" {
		return ErrCannotRenameMain
	}

	// Verify the source branch exists before any other check (including the
	// same-name no-op), so that a typo'd branch name is always reported rather
	// than silently treated as a successful no-op.
	oldRef, err := store.GetRef(ctx, "heads/"+oldName)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("check branch existence: %w", err)
		}
		return fmt.Errorf("branch '%s' not found: %w", oldName, ErrBranchNotFound)
	}

	if oldName == newName {
		return nil
	}

	// Validate the new name using the same rules as CreateBranch.
	if err := refname.Validate("heads/" + newName); err != nil {
		return fmt.Errorf("invalid branch name: %w", err)
	}

	if _, err := store.GetRef(ctx, "heads/"+newName); err == nil {
		return fmt.Errorf("branch '%s' already exists: %w", newName, ErrBranchAlreadyExists)
	} else if !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("check branch existence: %w", err)
	}

	// Create the new reference first. If this fails, the old one is intact.
	// Name stores the full ref path to match project.go convention and what
	// FSStorage.GetRef returns on read.
	newRef := &core.Reference{
		Type:   oldRef.Type,
		Name:   "heads/" + newName,
		Target: oldRef.Target,
	}
	if err := store.SetRef(ctx, "heads/"+newName, newRef); err != nil {
		return fmt.Errorf("create renamed branch: %w", err)
	}

	// Update HEAD if renaming the current branch.
	headRef, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}
	if headRef.SymRef == "heads/"+oldName {
		headRef.SymRef = "heads/" + newName
		if err := store.SetRef(ctx, "HEAD", headRef); err != nil {
			return fmt.Errorf("update HEAD: %w", err)
		}
	}

	// Finally remove the old reference.
	if err := store.DeleteRef(ctx, "heads/"+oldName); err != nil {
		return fmt.Errorf("remove old branch: %w", err)
	}

	return nil
}
