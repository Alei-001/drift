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

// ApplySummaryPagination applies ListOptions offset/limit pagination to a
// slice of snapshot summaries that has already been sorted by the caller.
// It is shared by the filesystem and memory backends so the pagination
// semantics (opts==nil → no pagination; Offset beyond len → nil; Limit
// truncation) stay identical across backends.
func ApplySummaryPagination(summaries []*core.SnapshotSummary, opts *ListOptions) []*core.SnapshotSummary {
	if opts == nil {
		return summaries
	}
	if opts.Offset > 0 {
		if opts.Offset >= len(summaries) {
			return nil
		}
		summaries = summaries[opts.Offset:]
	}
	if opts.Limit > 0 && opts.Limit < len(summaries) {
		summaries = summaries[:opts.Limit]
	}
	return summaries
}
