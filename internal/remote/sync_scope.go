package remote

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
)

// collectPushScope returns the set of snapshots, chunks and refs to push.
// When branch is non-empty only that branch's chain is included.
func collectPushScope(ctx context.Context, st *store.StoreSet, branch string) ([]core.SnapshotID, []core.Hash, []*core.Reference, error) {
	var snapIDs []core.SnapshotID
	var chunkHashes []core.Hash
	var refs []*core.Reference

	if branch != "" {
		// Branch-scoped: walk the branch's PrevID chain.
		refName := "heads/" + branch
		ref, err := st.Refs.GetRef(ctx, refName)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("get branch ref: %w", err)
		}
		refs = append(refs, ref)
		ids, chunks, err := walkSnapshotChain(ctx, st, core.SnapshotID{Hash: ref.Target})
		if err != nil {
			return nil, nil, nil, err
		}
		snapIDs = ids
		chunkHashes = chunks
	} else {
		// Full repo: list all snapshots, collect all refs.
		summaries, err := st.Snapshots.ListSnapshots(ctx, nil)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("list snapshots: %w", err)
		}
		seenChunks := make(map[core.Hash]bool)
		for _, s := range summaries {
			if err := ctx.Err(); err != nil {
				return nil, nil, nil, err
			}
			id := core.SnapshotID{Hash: s.ID.Hash}
			snapIDs = append(snapIDs, id)
			snap, err := st.Snapshots.GetSnapshot(ctx, id)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("get snapshot %s: %w", id.Hash.String(), err)
			}
			for _, f := range snap.Files {
				for _, ch := range f.Chunks {
					if !seenChunks[ch] {
						seenChunks[ch] = true
						chunkHashes = append(chunkHashes, ch)
					}
				}
			}
		}
		allRefs, err := st.Refs.ListRefs(ctx, "")
		if err != nil {
			return nil, nil, nil, fmt.Errorf("list refs: %w", err)
		}
		for _, r := range allRefs {
			if r.Name == "HEAD" {
				continue
			}
			refs = append(refs, r)
		}
	}
	return snapIDs, chunkHashes, refs, nil
}

// collectPullScope returns the set of snapshots, chunks and refs to pull.
// When branch is non-empty only that branch is included.
func collectPullScope(ctx context.Context, rfs RemoteFS, branch string) ([]core.SnapshotID, []core.Hash, []*core.Reference, error) {
	refs, err := listRemoteRefs(ctx, rfs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list remote refs: %w", err)
	}

	var snapIDs []core.SnapshotID
	var chunkHashes []core.Hash
	var refsToSync []*core.Reference

	if branch != "" {
		// Branch-scoped: only sync the specified branch.
		refName := "heads/" + branch
		var found *core.Reference
		for _, r := range refs {
			if r.Name == refName {
				found = r
				break
			}
		}
		if found == nil {
			return nil, nil, nil, fmt.Errorf("branch %q not found on remote: %w", branch, os.ErrNotExist)
		}
		refsToSync = append(refsToSync, found)
		ids, chunks, err := walkRemoteSnapshotChain(ctx, rfs, core.SnapshotID{Hash: found.Target})
		if err != nil {
			return nil, nil, nil, err
		}
		snapIDs = ids
		chunkHashes = chunks
	} else {
		// Full repo: list all remote manifests, collect all refs.
		ids, err := listRemoteSnapshots(ctx, rfs)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("list remote snapshots: %w", err)
		}
		snapIDs = ids
		seenChunks := make(map[core.Hash]bool)
		for _, id := range ids {
			if err := ctx.Err(); err != nil {
				return nil, nil, nil, err
			}
			snap, err := readRemoteSnapshot(ctx, rfs, id)
			if err != nil {
				return nil, nil, nil, err
			}
			for _, f := range snap.Files {
				for _, ch := range f.Chunks {
					if !seenChunks[ch] {
						seenChunks[ch] = true
						chunkHashes = append(chunkHashes, ch)
					}
				}
			}
		}
		refsToSync = refs
	}
	return snapIDs, chunkHashes, refsToSync, nil
}

// walkSnapshotChain walks the local PrevID chain from start and collects all
// reachable snapshot IDs and their chunk hashes.
func walkSnapshotChain(ctx context.Context, st *store.StoreSet, start core.SnapshotID) ([]core.SnapshotID, []core.Hash, error) {
	var snapIDs []core.SnapshotID
	var chunkHashes []core.Hash
	seen := make(map[core.Hash]bool)
	seenChunks := make(map[core.Hash]bool)
	cur := start
	for !cur.Hash.IsZero() {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if seen[cur.Hash] {
			break // cycle guard
		}
		seen[cur.Hash] = true
		snapIDs = append(snapIDs, cur)
		snap, err := st.Snapshots.GetSnapshot(ctx, cur)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				break
			}
			return nil, nil, fmt.Errorf("get snapshot %s: %w", cur.Hash.String(), err)
		}
		for _, f := range snap.Files {
			for _, ch := range f.Chunks {
				if !seenChunks[ch] {
					seenChunks[ch] = true
					chunkHashes = append(chunkHashes, ch)
				}
			}
		}
		if snap.PrevID == nil {
			break
		}
		cur = *snap.PrevID
	}
	return snapIDs, chunkHashes, nil
}

// walkRemoteSnapshotChain walks the remote PrevID chain from start.
func walkRemoteSnapshotChain(ctx context.Context, rfs RemoteFS, start core.SnapshotID) ([]core.SnapshotID, []core.Hash, error) {
	var snapIDs []core.SnapshotID
	var chunkHashes []core.Hash
	seen := make(map[core.Hash]bool)
	seenChunks := make(map[core.Hash]bool)
	cur := start
	for !cur.Hash.IsZero() {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if seen[cur.Hash] {
			break
		}
		seen[cur.Hash] = true
		snapIDs = append(snapIDs, cur)
		snap, err := readRemoteSnapshot(ctx, rfs, cur)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			return nil, nil, err
		}
		for _, f := range snap.Files {
			for _, ch := range f.Chunks {
				if !seenChunks[ch] {
					seenChunks[ch] = true
					chunkHashes = append(chunkHashes, ch)
				}
			}
		}
		if snap.PrevID == nil {
			break
		}
		cur = *snap.PrevID
	}
	return snapIDs, chunkHashes, nil
}

// isAncestor returns true if ancestor is reachable from descendant by walking
// the snapshot PrevID chain. Returns (false, nil) when ancestor is not
// reachable (the full chain was walked without finding it). Returns
// (false, err) when the chain cannot be fully walked — e.g. a snapshot in
// the descendant's chain is missing from the local st, or the context
// was cancelled. The caller should treat an error as "cannot determine
// ancestry" and fall back to the diverged/force-push path rather than
// silently assuming the ref is fast-forwardable.
func isAncestor(ctx context.Context, st *store.StoreSet, descendant, ancestor core.Hash) (bool, error) {
	if descendant.IsZero() || ancestor.IsZero() {
		return false, nil
	}
	if descendant == ancestor {
		return true, nil
	}
	cur := core.SnapshotID{Hash: descendant}
	seen := make(map[core.Hash]bool)
	for !cur.Hash.IsZero() {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if seen[cur.Hash] {
			break // cycle guard
		}
		seen[cur.Hash] = true
		if cur.Hash == ancestor {
			return true, nil
		}
		snap, err := st.Snapshots.GetSnapshot(ctx, cur)
		if err != nil {
			// A missing or unreadable snapshot breaks the chain: we
			// cannot determine whether ancestor is reachable. Surface
			// the error so the caller can distinguish "definitely not
			// an ancestor" from "cannot tell".
			return false, fmt.Errorf("walk snapshot chain at %s: %w", cur.Hash.String(), err)
		}
		if snap.PrevID == nil {
			break
		}
		cur = *snap.PrevID
	}
	return false, nil
}
