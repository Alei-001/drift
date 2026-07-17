package branch

import (
	"github.com/Alei-001/drift/internal/project"
	"github.com/Alei-001/drift/internal/errs"
	"github.com/Alei-001/drift/internal/snapshot"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
)

// SwitchBranch switches to the target branch. If create is true, it creates the branch first.
// It auto-saves current changes, updates HEAD symref, and restores the target snapshot to workspace.
// Returns autosave snapshot short ID (empty if nothing to save), the source branch name,
// and the number of files that differ between the source and target branch snapshots.
//
// When noAutosave is true, the auto-save step is skipped and the workspace
// must be clean (no uncommitted changes); otherwise errs.ErrUncommittedChanges is
// returned. This supports the 'drift switch --no-autosave' flow for users who
// have already manually saved and want to avoid an extra [auto] snapshot.
func SwitchBranch(ctx context.Context, st *store.StoreSet, workDir string, name string, create, noAutosave bool, author string, cfg *core.CoreConfig) (string, string, int, error) {
	if cfg == nil {
		cfg = &core.DefaultConfig().Core
	}
	if err := project.AcquireWorkspaceLock(workDir); err != nil {
		return "", "", 0, fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer project.ReleaseWorkspaceLock(workDir)

	headRef, err := st.Refs.GetRef(ctx, "HEAD")
	if err != nil {
		return "", "", 0, fmt.Errorf("read HEAD: %w", err)
	}
	fromBranch := ""
	if headRef.SymRef != "" {
		fromBranch = strings.TrimPrefix(headRef.SymRef, "heads/")
	}

	if create {
		if _, err := createBranchNoLock(ctx, st, name); err != nil {
			return "", "", 0, err
		}
	} else {
		if _, err := st.Refs.GetRef(ctx, "heads/"+name); err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				return "", "", 0, fmt.Errorf("check branch existence: %w", err)
			}
			return "", "", 0, fmt.Errorf("branch '%s' not found: %w", name, errs.ErrBranchNotFound)
		}
	}

	autosaveID := ""
	var autosaveSnap *core.Snapshot
	if noAutosave {
		summary, err := snapshot.DetectChangesNoLock(ctx, st, workDir, cfg)
		if err != nil {
			return "", "", 0, fmt.Errorf("check workspace changes: %w", err)
		}
		if len(summary.Added) > 0 || len(summary.Modified) > 0 || len(summary.Deleted) > 0 {
			return "", "", 0, errs.ErrUncommittedChanges
		}
	} else {
		autosaveSnap, err = snapshot.CreateSnapshotInLock(ctx, st, workDir, "auto - switch backup", author, cfg, false)
		if err != nil {
			if !errors.Is(err, errs.ErrNothingToSave) {
				return "", "", 0, fmt.Errorf("auto-save: %w", err)
			}
		} else {
			autosaveID = autosaveSnap.ShortID()
		}
	}

	// Read target branch ref (before any modification).
	targetRef, err := st.Refs.GetRef(ctx, "heads/"+name)
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
			if fromRef, refErr := st.Refs.GetRef(ctx, "heads/"+fromBranch); refErr == nil && fromRef != nil {
				sourceTarget = fromRef.Target
			}
		}
		if !sourceTarget.IsZero() {
			targetRef.Target = sourceTarget
			if err := st.Refs.SetRef(ctx, "heads/"+name, targetRef); err != nil {
				return "", "", 0, fmt.Errorf("init target branch: %w", err)
			}
		}
	}

	// Move HEAD to the target branch BEFORE restoring workspace files.
	// This makes HEAD the commit point: if SetRef fails, HEAD stays on
	// the source branch and the workspace is untouched — the user can
	// retry switch cleanly. If SetRef succeeds but workspace restore
	// fails, HEAD is on the target branch but the workspace is partially
	// modified; the user can re-run switch (idempotent: HEAD is already
	// on target, workspace gets re-restored) or run `drift restore` to
	// complete the transition.
	//
	// This order matches git's checkout: HEAD moves first (atomic ref
	// write), then the working tree is updated to match. The previous
	// order (workspace first, HEAD last) had a window where SetRef(HEAD)
	// failure left workspace=target but HEAD=source, causing the next
	// save to graft target content onto source history.
	newHeadRef := &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/" + name,
	}
	if err := st.Refs.SetRef(ctx, "HEAD", newHeadRef); err != nil {
		return "", "", 0, fmt.Errorf("update HEAD: %w", err)
	}

	// Restore target branch snapshot to workspace. Skip if target was
	// empty (workspace already matches the inherited snapshot via
	// auto-save, so restoring would be redundant).
	if !targetWasEmpty && !targetRef.Target.IsZero() {
		targetSnap, err := st.Snapshots.GetSnapshot(ctx, core.SnapshotID{Hash: targetRef.Target})
		if err != nil {
			return "", "", 0, fmt.Errorf("get target snapshot: %w", err)
		}
		if err := snapshot.RestoreFilesToWorkspace(ctx, st, workDir, cfg.IgnoreFile, targetSnap); err != nil {
			return "", "", 0, fmt.Errorf("restore workspace (HEAD already on %s; re-run switch or drift restore to complete): %w", name, err)
		}
	}

	var fromSnap, toSnap *core.Snapshot
	if fromBranch != "" {
		fromRef, refErr := st.Refs.GetRef(ctx, "heads/"+fromBranch)
		if refErr == nil && !fromRef.Target.IsZero() {
			snap, err := st.Snapshots.GetSnapshot(ctx, core.SnapshotID{Hash: fromRef.Target})
			if err != nil {
				return "", "", 0, fmt.Errorf("get source snapshot: %w", err)
			}
			fromSnap = snap
		}
	}
	if !targetRef.Target.IsZero() {
		snap, err := st.Snapshots.GetSnapshot(ctx, core.SnapshotID{Hash: targetRef.Target})
		if err != nil {
			return "", "", 0, fmt.Errorf("get target snapshot: %w", err)
		}
		toSnap = snap
	}
	diffCount := snapshot.CountSnapshotDiff(fromSnap, toSnap)

	slog.Info("branch switched", "from", fromBranch, "to", name, "created", create, "autosave", autosaveID, "diff_files", diffCount)

	return autosaveID, fromBranch, diffCount, nil
}
