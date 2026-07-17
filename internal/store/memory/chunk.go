package memory

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
	"github.com/zeebo/blake3"
)

// HasChunk returns whether a chunk exists.
func (ms *MemoryStorage) HasChunk(ctx context.Context, hash core.Hash) (bool, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	_, ok := ms.chunks[hash.FullString()]
	return ok, nil
}

// GetChunk retrieves a chunk.
func (ms *MemoryStorage) GetChunk(ctx context.Context, hash core.Hash) (*core.Chunk, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	v, ok := ms.chunks[hash.FullString()]
	if !ok {
		return nil, fmt.Errorf("get chunk %s: %w", hash.FullString(), store.ErrNotFound)
	}
	return store.CloneChunk(v), nil
}

// PutChunk stores a chunk. The chunk data is verified against its declared
// hash before storing, consistent with the filesystem backend, so a
// caller-supplied mismatch can never reach the store.
func (ms *MemoryStorage) PutChunk(ctx context.Context, chunk *core.Chunk) error {
	computed := core.Hash(blake3.Sum256(chunk.Data))
	if computed != chunk.Hash {
		return fmt.Errorf("put chunk %x: hash mismatch, expected %s, got %s: %w", chunk.Hash[:8], chunk.Hash.FullString(), computed.FullString(), store.ErrCorrupted)
	}
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.chunks[chunk.Hash.FullString()] = store.CloneChunk(chunk)
	return nil
}

// DeleteChunk removes a chunk. It is idempotent.
func (ms *MemoryStorage) DeleteChunk(ctx context.Context, hash core.Hash) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	delete(ms.chunks, hash.FullString())
	return nil
}

// ListChunks returns the hashes of all stored chunks. The order of the
// returned slice is not guaranteed.
func (ms *MemoryStorage) ListChunks(ctx context.Context) ([]core.Hash, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
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
