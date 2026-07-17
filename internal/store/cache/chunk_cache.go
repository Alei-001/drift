package cache

import (
	"context"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
	utilcache "github.com/Alei-001/drift/internal/util/cache"
)

// CachedChunkStore wraps a ChunkStorer with an LRU read cache. GetChunk
// hits the cache first; on a miss it delegates to the inner store and
// populates the cache. PutChunk and DeleteChunk invalidate the affected
// key. All other methods pass through to the inner store unchanged.
type CachedChunkStore struct {
	inner store.ChunkStorer
	cache *utilcache.Cache[core.Hash, *core.Chunk]
}

// NewCachedChunkStore creates a CachedChunkStore with a cache of the given
// size. It wraps inner — typically the filesystem backend — so that
// repeated reads of the same chunk (e.g. during diff or restore) avoid
// disk I/O and decompression.
func NewCachedChunkStore(inner store.ChunkStorer, size int) (*CachedChunkStore, error) {
	c, err := utilcache.NewCache[core.Hash, *core.Chunk](size)
	if err != nil {
		return nil, err
	}
	return &CachedChunkStore{inner: inner, cache: c}, nil
}

func (c *CachedChunkStore) HasChunk(ctx context.Context, hash core.Hash) (bool, error) {
	if _, ok := c.cache.Get(hash); ok {
		return true, nil
	}
	return c.inner.HasChunk(ctx, hash)
}

func (c *CachedChunkStore) GetChunk(ctx context.Context, hash core.Hash) (*core.Chunk, error) {
	if ch, ok := c.cache.Get(hash); ok {
		return store.CloneChunk(ch), nil
	}
	ch, err := c.inner.GetChunk(ctx, hash)
	if err != nil {
		return nil, err
	}
	c.cache.Add(hash, ch)
	return store.CloneChunk(ch), nil
}

func (c *CachedChunkStore) PutChunk(ctx context.Context, chunk *core.Chunk) error {
	c.cache.Remove(chunk.Hash)
	return c.inner.PutChunk(ctx, chunk)
}

func (c *CachedChunkStore) DeleteChunk(ctx context.Context, hash core.Hash) error {
	c.cache.Remove(hash)
	return c.inner.DeleteChunk(ctx, hash)
}

func (c *CachedChunkStore) ListChunks(ctx context.Context) ([]core.Hash, error) {
	return c.inner.ListChunks(ctx)
}

// Close is a no-op provided for symmetry with other store components.
func (c *CachedChunkStore) Close() error {
	return nil
}

// Ensure CachedChunkStore implements store.ChunkStorer + io.Closer.
var _ store.ChunkStorer = (*CachedChunkStore)(nil)
