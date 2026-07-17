package store

import (
	"context"

	"github.com/Alei-001/drift/internal/core"
)

// ChunkStorer provides access to chunk store.
type ChunkStorer interface {
	HasChunk(ctx context.Context, hash core.Hash) (bool, error)
	GetChunk(ctx context.Context, hash core.Hash) (*core.Chunk, error)
	PutChunk(ctx context.Context, chunk *core.Chunk) error
	DeleteChunk(ctx context.Context, hash core.Hash) error
	ListChunks(ctx context.Context) ([]core.Hash, error)
}
