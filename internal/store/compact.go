package store

import (
	"context"

	"github.com/Alei-001/drift/internal/core"
)

// ChunkCompactor is an optional interface implemented by storage backends
// that support compacting loose chunks into packed store. GC calls it
// after the reachability pass to pack loose chunks and rewrite packs with
// high dead-block ratios. Backends that do not support packing (e.g.
// in-memory storage) simply do not implement this interface; GC falls
// back to per-chunk DeleteChunk in that case.
type ChunkCompactor interface {
	CompactChunks(ctx context.Context, reachable map[core.Hash]bool, dryRun bool) (CompactReport, error)
}

// CompactReport describes the outcome of a chunk compaction pass.
type CompactReport struct {
	LooseDeleted    int
	LoosePacked     int
	PackDeadRemoved int
	PacksRewritten  int
	PacksCreated    int
	FreedBytes      int64
}
