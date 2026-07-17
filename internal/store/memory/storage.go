package memory

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
)

// Compile-time assertion that MemoryStorage implements store.Storer.
var _ store.Storer = (*MemoryStorage)(nil)

// MemoryStorage implements store.Storer entirely in memory.
// All methods are safe for concurrent use: a sync.RWMutex protects
// every map and pointer field. Read operations acquire a read lock;
// write operations acquire a write lock.
type MemoryStorage struct {
	mu        sync.RWMutex
	chunks    map[string]*core.Chunk
	snapshots map[string]*core.Snapshot
	manifests map[string]*core.SnapshotManifest
	refs      map[string]*core.Reference
	previews  map[string][]byte
	index     *core.Index
	config    *core.Config
}

// NewMemoryStorage creates a new in-memory store.
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		chunks:    make(map[string]*core.Chunk),
		snapshots: make(map[string]*core.Snapshot),
		manifests: make(map[string]*core.SnapshotManifest),
		refs:      make(map[string]*core.Reference),
		previews:  make(map[string][]byte),
		config:    core.DefaultConfig(),
	}
}

// Close releases any resources held by the memory store. It is a no-op.
func (ms *MemoryStorage) Close() error {
	return nil
}

// cloneManifest returns a deep copy of a SnapshotManifest.
// SnapshotManifest is a protobuf message (embeds protoimpl.MessageState
// which contains a sync.Mutex), so we must use proto.Clone rather than
// value copy to avoid copying the lock.
func cloneManifest(m *core.SnapshotManifest) *core.SnapshotManifest {
	if m == nil {
		return nil
	}
	return proto.Clone(m).(*core.SnapshotManifest)
}

// cloneReference returns a deep copy of a Reference.
// Hash is a value type ([32]byte), Name/Type are value types (string).
func cloneReference(r *core.Reference) *core.Reference {
	if r == nil {
		return nil
	}
	clone := *r
	return &clone
}

// cloneIndex returns a deep copy of an Index.
func cloneIndex(idx *core.Index) *core.Index {
	if idx == nil {
		return nil
	}
	clone := &core.Index{
		UpdatedAt: idx.UpdatedAt,
	}
	if idx.Entries != nil {
		clone.Entries = make([]core.IndexEntry, len(idx.Entries))
		for i, e := range idx.Entries {
			clone.Entries[i] = e
			if e.Chunks != nil {
				clone.Entries[i].Chunks = make([]core.Hash, len(e.Chunks))
				copy(clone.Entries[i].Chunks, e.Chunks)
			}
		}
	}
	return clone
}

// cloneConfig returns a deep copy of a Config.
// User and Core are value-type structs (no slices/maps/pointers).
func cloneConfig(c *core.Config) *core.Config {
	if c == nil {
		return nil
	}
	clone := *c
	return &clone
}

// GetIndex retrieves the staging index.
func (ms *MemoryStorage) GetIndex(ctx context.Context) (*core.Index, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.index == nil {
		return &core.Index{}, nil
	}
	return cloneIndex(ms.index), nil
}

// SetIndex stores the staging index.
func (ms *MemoryStorage) SetIndex(ctx context.Context, index *core.Index) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.index = cloneIndex(index)
	return nil
}

// GetPreview is a noop stub (Phase 1).
func (ms *MemoryStorage) GetPreview(ctx context.Context, hash core.Hash, size int) ([]byte, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return nil, fmt.Errorf("get preview %s: %w", hash.FullString(), store.ErrNotFound)
}

// PutPreview is a noop stub (Phase 1).
func (ms *MemoryStorage) PutPreview(ctx context.Context, hash core.Hash, size int, data []byte) error {
	return nil
}
