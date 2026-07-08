package porcelain

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
)

// minHashPrefixLen is the minimum number of hex characters required in an
// @id:<prefix> reference. Shorter prefixes are rejected to avoid accidental
// ambiguity as the snapshot count grows.
const minHashPrefixLen = 4

// ResolveSnapshotRef resolves a snapshot reference to a snapshot.
//
// Snapshot reference syntax (see docs/cli-design.md "版本引用语法"):
//   - @id:<hash-prefix> — match by snapshot hash prefix (>= minHashPrefixLen chars)
//   - @tag:<name>       — resolve via tags/<name> reference
//   - @branch:<name>    — resolve via heads/<name> reference (branch head)
//   - @head             — current HEAD snapshot
//   - <bare-name>       — equivalent to @branch:<bare-name>
//
// Returns ErrSnapshotNotFound if the referenced snapshot does not exist.
// Returns an error wrapping ErrAmbiguousID if the hash prefix matches more
// than one snapshot (the message lists the matching short IDs).
// Returns an error if the hash prefix is shorter than minHashPrefixLen.
func ResolveSnapshotRef(ctx context.Context, store storage.Storer, id string) (*core.Snapshot, error) {
	switch {
	case id == "@head":
		return resolveHead(ctx, store)
	case strings.HasPrefix(id, "@id:"):
		return resolveByID(ctx, store, id[4:])
	case strings.HasPrefix(id, "@tag:"):
		return resolveByRef(ctx, store, "tags/"+id[5:])
	case strings.HasPrefix(id, "@branch:"):
		return resolveByRef(ctx, store, "heads/"+id[8:])
	case strings.HasPrefix(id, "@"):
		// Unknown @ prefix — no match.
		return nil, fmt.Errorf("unknown snapshot reference %q: %w", id, ErrSnapshotNotFound)
	default:
		// Bare name is equivalent to @branch:<name>. Branch names are
		// user-chosen readable names and never collide with machine-generated
		// hashes, so bare names are unambiguous.
		return resolveByRef(ctx, store, "heads/"+id)
	}
}

// resolveHead resolves the HEAD reference to a snapshot, following symbolic
// references. Returns ErrSnapshotNotFound when HEAD is missing or points to
// nothing.
func resolveHead(ctx context.Context, store storage.Storer) (*core.Snapshot, error) {
	headRef, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("read HEAD: %w", ErrSnapshotNotFound)
	}
	target := headRef.Target
	if headRef.SymRef != "" {
		branchRef, err := store.GetRef(ctx, headRef.SymRef)
		if err != nil {
			return nil, fmt.Errorf("read HEAD symref %q: %w", headRef.SymRef, ErrSnapshotNotFound)
		}
		target = branchRef.Target
	}
	if target.IsZero() {
		return nil, fmt.Errorf("HEAD points at nothing: %w", ErrSnapshotNotFound)
	}
	snap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: target})
	if err != nil {
		return nil, fmt.Errorf("load HEAD snapshot %s: %w", target, ErrSnapshotNotFound)
	}
	return snap, nil
}

// resolveByID resolves a snapshot by hash prefix. The prefix must be at least
// minHashPrefixLen characters. Returns ErrSnapshotNotFound when no snapshot
// matches, or an error wrapping ErrAmbiguousID when more than one snapshot
// matches.
func resolveByID(ctx context.Context, store storage.Storer, prefix string) (*core.Snapshot, error) {
	if len(prefix) < minHashPrefixLen {
		return nil, fmt.Errorf("snapshot ID prefix %q is too short (minimum %d characters)", prefix, minHashPrefixLen)
	}
	summaries, err := store.ListSnapshots(ctx, &storage.ListOptions{})
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
		return nil, fmt.Errorf("snapshot ID %q: %w", prefix, ErrSnapshotNotFound)
	}
	if len(matches) > 1 {
		var shortIDs []string
		for _, m := range matches {
			shortIDs = append(shortIDs, m.ShortID())
		}
		return nil, fmt.Errorf("ambiguous snapshot ID %q matches %d snapshots [%s]: %w",
			prefix, len(matches), strings.Join(shortIDs, ", "), ErrAmbiguousID)
	}
	snap, err := store.GetSnapshot(ctx, matches[0].ID)
	if err != nil {
		return nil, fmt.Errorf("load snapshot %s: %w", matches[0].ShortID(), ErrSnapshotNotFound)
	}
	return snap, nil
}

// resolveByRef resolves a snapshot via a named reference (e.g. "heads/main",
// "tags/v1"). Returns ErrSnapshotNotFound when the reference or its target
// snapshot is missing.
func resolveByRef(ctx context.Context, store storage.Storer, refName string) (*core.Snapshot, error) {
	ref, err := store.GetRef(ctx, refName)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("reference %q: %w", refName, ErrSnapshotNotFound)
		}
		return nil, fmt.Errorf("read reference %q: %w", refName, err)
	}
	if ref.Target.IsZero() {
		return nil, fmt.Errorf("reference %q points at nothing: %w", refName, ErrSnapshotNotFound)
	}
	snap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: ref.Target})
	if err != nil {
		return nil, fmt.Errorf("load snapshot %s: %w", ref.Target, ErrSnapshotNotFound)
	}
	return snap, nil
}
