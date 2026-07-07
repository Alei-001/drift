package memory

import (
	"google.golang.org/protobuf/proto"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
)

// Compile-time assertion that MemoryStorage implements storage.Storer.
var _ storage.Storer = (*MemoryStorage)(nil)

// MemoryStorage implements storage.Storer entirely in memory.
// It assumes single-threaded access: the porcelain workspace lock
// guarantees that no two goroutines call its methods concurrently.
type MemoryStorage struct {
	chunks    map[string]*core.Chunk
	snapshots map[string]*core.Snapshot
	manifests map[string]*core.SnapshotManifest
	refs      map[string]*core.Reference
	previews  map[string][]byte
	index     *core.Index
	config    *core.Config
}

// NewMemoryStorage creates a new in-memory storage.
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

// Close releases any resources held by the memory storage. It is a no-op.
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
