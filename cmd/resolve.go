package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/Alei-001/drift/internal/storage"
)

// resolveSnapshot resolves a snapshot reference to a snapshot.
//
// Snapshot reference syntax (see docs/cli-design.md "版本引用语法"):
//   - @id:<hash-prefix> — match by snapshot hash prefix (>= 4 chars)
//   - @tag:<name>       — resolve via tags/<name> reference
//   - @branch:<name>    — resolve via heads/<name> reference (branch head)
//   - @head             — current HEAD snapshot
//   - <bare-name>       — equivalent to @branch:<bare-name>
//
// Returns nil if the snapshot is not found or the hash prefix is ambiguous.
// Ambiguous-prefix details are printed to stderr to match the historical
// behavior. The caller is responsible for reporting a user-facing error on nil.
//
// This is a thin wrapper over porcelain.ResolveSnapshotRef so that existing
// callers (which expect a *core.Snapshot with nil signalling "not found")
// do not need to change. New code should call porcelain.ResolveSnapshotRef
// directly to inspect the error.
func resolveSnapshot(ctx context.Context, store storage.Storer, id string) *core.Snapshot {
	snap, err := porcelain.ResolveSnapshotRef(ctx, store, id)
	if err != nil {
		if errors.Is(err, porcelain.ErrAmbiguousID) {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		return nil
	}
	return snap
}
