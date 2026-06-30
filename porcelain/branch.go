package porcelain

import (
	"context"
	"fmt"
	"strings"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage"
)

// CreateBranch creates a new branch pointing at the current HEAD snapshot.
func CreateBranch(store storage.Storer, name string) error {
	ctx := context.Background()
	if name == "" {
		return fmt.Errorf("branch name is empty")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid branch name: %q contains '..'", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid branch name: %q contains path separator", name)
	}

	if ref, err := store.GetRef(ctx, "heads/"+name); err == nil && ref != nil {
		return fmt.Errorf("branch '%s' already exists", name)
	}

	headRef, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
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
func ListBranches(store storage.Storer) ([]*core.Reference, string, error) {
	ctx := context.Background()
	refs, err := store.ListRefs(ctx, "heads/")
	if err != nil {
		return nil, "", err
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

// SwitchBranch switches to the target branch. If create is true, it creates the branch first.
// It auto-saves current changes, updates HEAD symref, and restores the target snapshot to workspace.
// Returns autosave snapshot short ID (empty if nothing to save), the source branch name,
// and the number of files that differ between the source and target branch snapshots.
func SwitchBranch(store storage.Storer, workDir string, name string, create bool, author string) (string, string, int, error) {
	ctx := context.Background()
	if err := AcquireWorkspaceLock(workDir); err != nil {
		return "", "", 0, err
	}
	defer ReleaseWorkspaceLock(workDir)

	headRef, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		return "", "", 0, fmt.Errorf("read HEAD: %w", err)
	}
	fromBranch := ""
	if headRef.SymRef != "" {
		fromBranch = strings.TrimPrefix(headRef.SymRef, "heads/")
	}

	if create {
		if err := CreateBranch(store, name); err != nil {
			return "", "", 0, err
		}
	} else {
		if _, err := store.GetRef(ctx, "heads/"+name); err != nil {
			return "", "", 0, fmt.Errorf("branch '%s' not found", name)
		}
	}

	autosaveID := ""
	autosaveSnap, err := CreateSnapshot(store, workDir, "auto - switch backup", author, nil)
	if err != nil {
		if err.Error() != "nothing to save" {
			return "", "", 0, fmt.Errorf("auto-save: %w", err)
		}
	} else {
		autosaveID = autosaveSnap.ShortID()
	}

	// Read target branch ref (before any modification).
	targetRef, err := store.GetRef(ctx, "heads/"+name)
	if err != nil {
		return "", "", 0, fmt.Errorf("read target branch: %w", err)
	}
	targetWasEmpty := targetRef.Target.IsZero()

	// If target branch is empty (no commits), inherit the source branch's
	// current snapshot as the target's initial state. This makes the first
	// save on the target branch have a parent snapshot, so the diff display
	// is meaningful (only actual changes are shown, not all files).
	// Behavior mirrors git: a branch switched to from another branch shares
	// the source's history as its starting point.
	if targetWasEmpty {
		var sourceTarget core.Hash
		if autosaveSnap != nil {
			sourceTarget = autosaveSnap.ID.Hash
		} else if fromBranch != "" {
			if fromRef, refErr := store.GetRef(ctx, "heads/"+fromBranch); refErr == nil && fromRef != nil {
				sourceTarget = fromRef.Target
			}
		}
		if !sourceTarget.IsZero() {
			targetRef.Target = sourceTarget
			if err := store.SetRef(ctx, "heads/"+name, targetRef); err != nil {
				return "", "", 0, fmt.Errorf("init target branch: %w", err)
			}
		}
	}

	newHeadRef := &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/" + name,
	}
	if err := store.SetRef(ctx, "HEAD", newHeadRef); err != nil {
		return "", "", 0, fmt.Errorf("update HEAD: %w", err)
	}

	// Restore target branch snapshot to workspace. Skip if target was empty
	// (workspace already matches the inherited snapshot via auto-save, so
	// restoring would be redundant).
	if !targetWasEmpty && !targetRef.Target.IsZero() {
		targetSnap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: targetRef.Target})
		if err != nil {
			return "", "", 0, fmt.Errorf("get target snapshot: %w", err)
		}
		if err := restoreFilesToWorkspace(ctx, store, workDir, targetSnap); err != nil {
			return "", "", 0, fmt.Errorf("restore workspace: %w", err)
		}
	}

	var fromSnap, toSnap *core.Snapshot
	if fromBranch != "" {
		fromRef, refErr := store.GetRef(ctx, "heads/"+fromBranch)
		if refErr == nil && !fromRef.Target.IsZero() {
			fromSnap, _ = store.GetSnapshot(ctx, core.SnapshotID{Hash: fromRef.Target})
		}
	}
	if !targetRef.Target.IsZero() {
		toSnap, _ = store.GetSnapshot(ctx, core.SnapshotID{Hash: targetRef.Target})
	}
	diffCount := countSnapshotDiff(fromSnap, toSnap)

	return autosaveID, fromBranch, diffCount, nil
}

func countSnapshotDiff(from, to *core.Snapshot) int {
	if from == nil && to == nil {
		return 0
	}
	if from == nil {
		return len(to.Files)
	}
	if to == nil {
		return len(from.Files)
	}
	fromFiles := make(map[string]core.FileEntry)
	for _, f := range from.Files {
		fromFiles[f.Path] = f
	}
	count := 0
	seen := make(map[string]bool)
	for _, f := range to.Files {
		seen[f.Path] = true
		if prev, ok := fromFiles[f.Path]; !ok {
			count++
		} else if prev.Hash != f.Hash {
			count++
		}
	}
	for p := range fromFiles {
		if !seen[p] {
			count++
		}
	}
	return count
}

// DeleteBranch removes a branch reference. It refuses to delete:
//   - "main" (the default, protected branch)
//   - the current branch (user must switch away first)
//
// Only the reference is removed; snapshots remain in storage and can be
// reclaimed later by a future prune/GC command.
func DeleteBranch(store storage.Storer, name string) error {
	ctx := context.Background()
	if name == "" {
		return fmt.Errorf("branch name is empty")
	}
	if name == "main" {
		return fmt.Errorf("cannot delete 'main'")
	}

	if _, err := store.GetRef(ctx, "heads/"+name); err != nil {
		return fmt.Errorf("branch '%s' not found", name)
	}

	headRef, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		return fmt.Errorf("read HEAD: %w", err)
	}
	if headRef.SymRef == "heads/"+name {
		return fmt.Errorf("cannot delete the current branch '%s'", name)
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
func RenameBranch(store storage.Storer, oldName, newName string) error {
	ctx := context.Background()
	if oldName == "" {
		return fmt.Errorf("old branch name is empty")
	}
	if newName == "" {
		return fmt.Errorf("new branch name is empty")
	}
	if oldName == "main" {
		return fmt.Errorf("cannot rename 'main'")
	}

	// Verify the source branch exists before any other check (including the
	// same-name no-op), so that a typo'd branch name is always reported rather
	// than silently treated as a successful no-op.
	oldRef, err := store.GetRef(ctx, "heads/"+oldName)
	if err != nil {
		return fmt.Errorf("branch '%s' not found", oldName)
	}

	if oldName == newName {
		return nil
	}

	// Validate the new name using the same rules as CreateBranch.
	if strings.Contains(newName, "..") {
		return fmt.Errorf("invalid branch name: %q contains '..'", newName)
	}
	if strings.ContainsAny(newName, `/\`) {
		return fmt.Errorf("invalid branch name: %q contains path separator", newName)
	}

	if _, err := store.GetRef(ctx, "heads/"+newName); err == nil {
		return fmt.Errorf("branch '%s' already exists", newName)
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
