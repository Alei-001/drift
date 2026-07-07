package memory

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
)

// HasChunk returns whether a chunk exists.
func (ms *MemoryStorage) HasChunk(ctx context.Context, hash core.Hash) (bool, error) {
	_, ok := ms.chunks[hash.FullString()]
	return ok, nil
}

// GetChunk retrieves a chunk.
func (ms *MemoryStorage) GetChunk(ctx context.Context, hash core.Hash) (*core.Chunk, error) {
	v, ok := ms.chunks[hash.FullString()]
	if !ok {
		return nil, fmt.Errorf("get chunk %s: %w", hash.FullString(), storage.ErrNotFound)
	}
	return storage.CloneChunk(v), nil
}

// PutChunk stores a chunk.
func (ms *MemoryStorage) PutChunk(ctx context.Context, chunk *core.Chunk) error {
	ms.chunks[chunk.Hash.FullString()] = storage.CloneChunk(chunk)
	return nil
}

// DeleteChunk removes a chunk. It is idempotent.
func (ms *MemoryStorage) DeleteChunk(ctx context.Context, hash core.Hash) error {
	delete(ms.chunks, hash.FullString())
	return nil
}

// ListChunks returns the hashes of all stored chunks. The order of the
// returned slice is not guaranteed.
func (ms *MemoryStorage) ListChunks(ctx context.Context) ([]core.Hash, error) {
	var hashes []core.Hash
	for key := range ms.chunks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		b, err := hex.DecodeString(key)
		if err != nil {
			continue
		}
		var h core.Hash
		copy(h[:], b)
		hashes = append(hashes, h)
	}
	return hashes, nil
}
