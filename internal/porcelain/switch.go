package porcelain

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
)

// SwitchBranch switches to the target branch. If create is true, it creates the branch first.
// It auto-saves current changes, updates HEAD symref, and restores the target snapshot to workspace.
// Returns autosave snapshot short ID (empty if nothing to save), the source branch name,
// and the number of files that differ between the source and target branch snapshots.
//
// When noAutosave is true, the auto-save step is skipped and the workspace
// must be clean (no uncommitted changes); otherwise ErrUncommittedChanges is
// returned. This supports the 'drift switch --no-autosave' flow for users who
// have already manually saved and want to avoid an extra [auto] snapshot.
func SwitchBranch(ctx context.Context, store storage.Storer, workDir string, name string, create, noAutosave bool, author string, cfg *core.CoreConfig) (string, string, int, error) {
	if cfg == nil {
		cfg = &core.DefaultConfig().Core
	}
	if err := AcquireWorkspaceLock(workDir); err != nil {
		return "", "", 0, fmt.Errorf("acquire workspace lock: %w", err)
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
		if err := createBranchNoLock(ctx, store, name); err != nil {
			return "", "", 0, err
		}
	} else {
		if _, err := store.GetRef(ctx, "heads/"+name); err != nil {
			if !errors.Is(err, storage.ErrNotFound) {
				return "", "", 0, fmt.Errorf("check branch existence: %w", err)
			}
			return "", "", 0, fmt.Errorf("branch '%s' not found: %w", name, ErrBranchNotFound)
		}
	}

	autosaveID := ""
	var autosaveSnap *core.Snapshot
	if noAutosave {
		summary, err := detectChangesNoLock(ctx, store, workDir, cfg)
		if err != nil {
			return "", "", 0, fmt.Errorf("check workspace changes: %w", err)
		}
		if len(summary.Added) > 0 || len(summary.Modified) > 0 || len(summary.Deleted) > 0 {
			return "", "", 0, ErrUncommittedChanges
		}
	} else {
		autosaveSnap, err = createSnapshotInLock(ctx, store, workDir, "auto - switch backup", author, cfg)
		if err != nil {
			if !errors.Is(err, ErrNothingToSave) {
				return "", "", 0, fmt.Errorf("auto-save: %w", err)
			}
		} else {
			autosaveID = autosaveSnap.ShortID()
		}
	}

	// Read target branch ref (before any modification).
	targetRef, err := store.GetRef(ctx, "heads/"+name)
	if err != nil {
		return "", "", 0, fmt.Errorf("read target branch: %w", err)
	}
	targetWasEmpty := targetRef.Target.IsZero()

	// Detached HEAD switching to an empty branch with no changes to save
	// would leave the target branch pointing at a zero hash: autosave has
	// nothing to link (autosaveSnap == nil) and there is no source branch
	// to inherit from (SymRef == ""). The next save on such a branch would
	// create a root snapshot (PrevID=nil), severing the workspace's history
	// from any prior snapshots. Refuse the switch so the user can either
	// save first (which links via autosave) or switch to a non-empty branch.
	if headRef.SymRef == "" && targetWasEmpty && autosaveSnap == nil {
		return "", "", 0, fmt.Errorf("cannot switch to empty branch from detached HEAD without changes to save")
	}

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
		if err := restoreFilesToWorkspace(ctx, store, workDir, cfg.IgnoreFile, targetSnap); err != nil {
			return "", "", 0, fmt.Errorf("restore workspace: %w", err)
		}
	}

	var fromSnap, toSnap *core.Snapshot
	if fromBranch != "" {
		fromRef, refErr := store.GetRef(ctx, "heads/"+fromBranch)
		if refErr == nil && !fromRef.Target.IsZero() {
			snap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: fromRef.Target})
			if err != nil {
				return "", "", 0, fmt.Errorf("get source snapshot: %w", err)
			}
			fromSnap = snap
		}
	}
	if !targetRef.Target.IsZero() {
		snap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: targetRef.Target})
		if err != nil {
			return "", "", 0, fmt.Errorf("get target snapshot: %w", err)
		}
		toSnap = snap
	}
	diffCount := countSnapshotDiff(fromSnap, toSnap)

	return autosaveID, fromBranch, diffCount, nil
}
