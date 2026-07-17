package remote

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
)

// rebuildIndex reconstructs the local index from the given snapshot tip.
// Called after a pull that advances the current branch.
func rebuildIndex(ctx context.Context, st *store.StoreSet, tip core.SnapshotID) error {
	snap, err := st.Snapshots.GetSnapshot(ctx, tip)
	if err != nil {
		return fmt.Errorf("get snapshot: %w", err)
	}
	newIndex := &core.Index{UpdatedAt: time.Now().Unix()}
	for _, entry := range snap.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		newIndex.Entries = append(newIndex.Entries, core.IndexEntry{
			Path:    entry.Path,
			Size:    entry.Size,
			ModTime: entry.ModTime,
			Chunks:  entry.Chunks,
			Hash:    entry.Hash,
		})
	}
	return st.Index.SetIndex(ctx, newIndex)
}

// currentBranchName returns the current branch name (e.g. "main"), or "" on
// error or detached HEAD. For internal use within the remote package only.
func currentBranchName(ctx context.Context, st *store.StoreSet) string {
	head, err := st.Refs.GetRef(ctx, "HEAD")
	if err != nil || head.SymRef == "" {
		return ""
	}
	return strings.TrimPrefix(head.SymRef, "heads/")
}

// currentBranchTip resolves HEAD → branch ref → snapshot ID.
// Returns a zero SnapshotID on error (e.g. fresh repo with no commits).
func currentBranchTip(ctx context.Context, st *store.StoreSet) (core.SnapshotID, error) {
	head, err := st.Refs.GetRef(ctx, "HEAD")
	if err != nil || head.SymRef == "" {
		return core.SnapshotID{}, fmt.Errorf("HEAD is not a symbolic ref")
	}
	ref, err := st.Refs.GetRef(ctx, head.SymRef)
	if err != nil {
		return core.SnapshotID{}, err
	}
	return core.SnapshotID{Hash: ref.Target}, nil
}
