package store

import "github.com/Alei-001/drift/internal/core"

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
