package storage

import (
	"context"

	"github.com/your-org/drift/core"
)

// ListOptions controls snapshot listing pagination and filtering.
type ListOptions struct {
	Limit  int
	Offset int
	Branch string
}

// SnapshotStorer provides access to snapshot storage.
type SnapshotStorer interface {
	GetSnapshot(ctx context.Context, id core.SnapshotID) (*core.Snapshot, error)
	PutSnapshot(ctx context.Context, snapshot *core.Snapshot) error
	DeleteSnapshot(ctx context.Context, id core.SnapshotID) error
	ListSnapshots(ctx context.Context, opts *ListOptions) ([]*core.Snapshot, error)
}
