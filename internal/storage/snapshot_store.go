package storage

import (
	"context"

	"github.com/Alei-001/drift/internal/core"
)

// ListOptions controls snapshot listing pagination.
type ListOptions struct {
	Limit  int
	Offset int
}

// SnapshotStorer provides access to snapshot storage.
type SnapshotStorer interface {
	GetSnapshot(ctx context.Context, id core.SnapshotID) (*core.Snapshot, error)
	PutSnapshot(ctx context.Context, snapshot *core.Snapshot) error
	DeleteSnapshot(ctx context.Context, id core.SnapshotID) error
	ListSnapshots(ctx context.Context, opts *ListOptions) ([]*core.SnapshotSummary, error)
}
