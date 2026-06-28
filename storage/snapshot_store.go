package storage

import "github.com/your-org/drift/core"

// ListOptions controls snapshot listing pagination and filtering.
type ListOptions struct {
	Limit  int
	Offset int
	Branch string
}

// SnapshotStorer provides access to snapshot storage.
type SnapshotStorer interface {
	GetSnapshot(id core.SnapshotID) (*core.Snapshot, error)
	PutSnapshot(snapshot *core.Snapshot) error
	ListSnapshots(opts *ListOptions) ([]*core.Snapshot, error)
}
