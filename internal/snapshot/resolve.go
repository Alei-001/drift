package snapshot

import (
	"github.com/Alei-001/drift/internal/errs"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
)

// minHashPrefixLen is the minimum number of hex characters required in an
// id:<prefix> reference. Shorter prefixes are rejected to avoid accidental
// ambiguity as the snapshot count grows.
const minHashPrefixLen = 4

// ResolveSnapshotRef resolves a snapshot reference to a snapshot.
//
// Snapshot reference syntax (see docs/cli-design.md "版本引用语法"):
//   - id:<hash-prefix>  — match by snapshot hash prefix (>= minHashPrefixLen chars)
//   - tag:<name>        — resolve via tags/<name> reference
//   - branch:<name>     — resolve via heads/<name> reference (branch head)
//   - head              — current HEAD snapshot
//   - <bare-name>       — equivalent to branch:<bare-name>
//
// The colon-prefixed syntax replaces the earlier @-prefixed form (@id:...,
// @tag:..., @branch:..., @head) which collided with PowerShell's splat
// operator and required quoting on Windows. Colons are ordinary characters
// in all common shells (PowerShell, bash, zsh, fish, cmd) and are already
// rejected by refname.Validate, so they cannot appear in branch or tag names.
//
// Returns errs.ErrSnapshotNotFound if the referenced snapshot does not exist.
// Returns an error wrapping errs.ErrAmbiguousID if the hash prefix matches more
// than one snapshot (the message lists the matching short IDs).
// Returns an error if the hash prefix is shorter than minHashPrefixLen.
func ResolveSnapshotRef(ctx context.Context, st *store.StoreSet, id string) (*core.Snapshot, error) {
	switch {
	case id == "head":
		return resolveHead(ctx, st)
	case strings.HasPrefix(id, "id:"):
		return resolveByID(ctx, st, id[3:])
	case strings.HasPrefix(id, "tag:"):
		return resolveByRef(ctx, st, "tags/"+id[4:])
	case strings.HasPrefix(id, "branch:"):
		return resolveByRef(ctx, st, "heads/"+id[7:])
	default:
		// Bare name is equivalent to branch:<name>. Branch names are
		// user-chosen readable names and never collide with machine-generated
		// hashes, so bare names are unambiguous. refname.Validate rejects
		// "head" as a reserved keyword, so bare "head" is always the keyword.
		return resolveByRef(ctx, st, "heads/"+id)
	}
}

// resolveHead resolves the HEAD reference to a snapshot, following symbolic
// references.
//
// Error classification:
//   - HEAD ref missing  → errs.ErrNotARepo (the workspace has not been
//     initialized; `drift log` in such a directory should exit with
//     ExitNotRepo, not ExitInternalError).
//   - HEAD present but Target is zero (empty repo with no commits yet)
//     → errs.ErrSnapshotNotFound (the repo exists but there is no snapshot to
//     show).
//   - HEAD points at a snapshot that is not in the store (corruption)
//     → errs.ErrSnapshotNotFound (wrapped with the missing hash for context).
//
// store.GetRef is contractually required to fully resolve symrefs: a GetRef
// on a symref HEAD returns a Reference whose Target is the final snapshot
// hash (not the intermediate branch ref). This matches the documented
// contract of *store.StoreSet.GetRef and the filesystem backend's
// implementation in internal/storage/backends/filesystem/ref.go. Callers
// therefore never need a second GetRef to chase symrefs. ResolveHeadSnapshot
// (in snapshot_branch.go) relies on the same contract; the two functions
// intentionally use a single GetRef.
func resolveHead(ctx context.Context, st *store.StoreSet) (*core.Snapshot, error) {
	headRef, err := st.Refs.GetRef(ctx, "HEAD")
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("read HEAD: %w", errs.ErrNotARepo)
		}
		return nil, fmt.Errorf("read HEAD: %w", err)
	}
	if headRef.Target.IsZero() {
		return nil, fmt.Errorf("HEAD points at nothing: %w", errs.ErrSnapshotNotFound)
	}
	snap, err := st.Snapshots.GetSnapshot(ctx, core.SnapshotID{Hash: headRef.Target})
	if err != nil {
		return nil, fmt.Errorf("load HEAD snapshot %s: %w", headRef.Target, errs.ErrSnapshotNotFound)
	}
	return snap, nil
}

// resolveByID resolves a snapshot by hash prefix. The prefix must be at least
// minHashPrefixLen characters. Returns errs.ErrSnapshotNotFound when no snapshot
// matches, or an error wrapping errs.ErrAmbiguousID when more than one snapshot
// matches.
func resolveByID(ctx context.Context, st *store.StoreSet, prefix string) (*core.Snapshot, error) {
	if len(prefix) < minHashPrefixLen {
		return nil, fmt.Errorf("snapshot ID prefix %q is too short (minimum %d characters)", prefix, minHashPrefixLen)
	}
	summaries, err := st.Snapshots.ListSnapshots(ctx, &store.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	var matches []*core.SnapshotSummary
	for _, s := range summaries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if strings.HasPrefix(s.ShortID(), prefix) || strings.HasPrefix(s.FullID(), prefix) {
			matches = append(matches, s)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("snapshot ID %q: %w", prefix, errs.ErrSnapshotNotFound)
	}
	if len(matches) > 1 {
		var shortIDs []string
		for _, m := range matches {
			shortIDs = append(shortIDs, m.ShortID())
		}
		return nil, fmt.Errorf("ambiguous snapshot ID %q matches %d snapshots [%s]: %w",
			prefix, len(matches), strings.Join(shortIDs, ", "), errs.ErrAmbiguousID)
	}
	snap, err := st.Snapshots.GetSnapshot(ctx, matches[0].ID)
	if err != nil {
		return nil, fmt.Errorf("load snapshot %s: %w", matches[0].ShortID(), errs.ErrSnapshotNotFound)
	}
	return snap, nil
}

// resolveByRef resolves a snapshot via a named reference (e.g. "heads/main",
// "tags/v1"). Returns errs.ErrSnapshotNotFound when the reference or its target
// snapshot is missing.
func resolveByRef(ctx context.Context, st *store.StoreSet, refName string) (*core.Snapshot, error) {
	ref, err := st.Refs.GetRef(ctx, refName)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("reference %q: %w", refName, errs.ErrSnapshotNotFound)
		}
		return nil, fmt.Errorf("read reference %q: %w", refName, err)
	}
	if ref.Target.IsZero() {
		return nil, fmt.Errorf("reference %q points at nothing: %w", refName, errs.ErrSnapshotNotFound)
	}
	snap, err := st.Snapshots.GetSnapshot(ctx, core.SnapshotID{Hash: ref.Target})
	if err != nil {
		return nil, fmt.Errorf("load snapshot %s: %w", ref.Target, errs.ErrSnapshotNotFound)
	}
	return snap, nil
}
