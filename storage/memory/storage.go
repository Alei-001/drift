package memory

import (
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage"
)

// MemoryStorage implements storage.Storer entirely in memory.
type MemoryStorage struct {
	chunks    sync.Map
	snapshots sync.Map
	refs      sync.Map
	previews  sync.Map
	mu        sync.RWMutex
	index     *core.Index
	config    *core.Config
}

// NewMemoryStorage creates a new in-memory storage.
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		config: core.DefaultConfig(),
	}
}

// HasChunk returns whether a chunk exists.
func (ms *MemoryStorage) HasChunk(hash core.Hash) bool {
	_, ok := ms.chunks.Load(hash.FullString())
	return ok
}

// GetChunk retrieves a chunk.
func (ms *MemoryStorage) GetChunk(hash core.Hash) (*core.Chunk, error) {
	v, ok := ms.chunks.Load(hash.FullString())
	if !ok {
		return nil, errors.New("chunk not found")
	}
	return v.(*core.Chunk), nil
}

// PutChunk stores a chunk.
func (ms *MemoryStorage) PutChunk(chunk *core.Chunk) error {
	ms.chunks.Store(chunk.Hash.FullString(), chunk)
	return nil
}

// GetSnapshot retrieves a snapshot.
func (ms *MemoryStorage) GetSnapshot(id core.SnapshotID) (*core.Snapshot, error) {
	v, ok := ms.snapshots.Load(id.Hash.FullString())
	if !ok {
		return nil, errors.New("snapshot not found")
	}
	return v.(*core.Snapshot), nil
}

// PutSnapshot stores a snapshot.
func (ms *MemoryStorage) PutSnapshot(snapshot *core.Snapshot) error {
	ms.snapshots.Store(snapshot.ID.Hash.FullString(), snapshot)
	return nil
}

// ListSnapshots lists all snapshots, sorted by timestamp descending,
// with optional limit/offset and branch filter.
func (ms *MemoryStorage) ListSnapshots(opts *storage.ListOptions) ([]*core.Snapshot, error) {
	var snapshots []*core.Snapshot
	ms.snapshots.Range(func(key, value any) bool {
		snapshots = append(snapshots, value.(*core.Snapshot))
		return true
	})

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp > snapshots[j].Timestamp
	})

	if opts == nil {
		return snapshots, nil
	}

	if opts.Branch != "" {
		branchFilter := opts.Branch
		filtered := make([]*core.Snapshot, 0, len(snapshots))
		for _, s := range snapshots {
			for _, t := range s.Tags {
				if t == branchFilter {
					filtered = append(filtered, s)
					break
				}
			}
		}
		snapshots = filtered
	}

	if opts.Offset > 0 {
		if opts.Offset >= len(snapshots) {
			return nil, nil
		}
		snapshots = snapshots[opts.Offset:]
	}
	if opts.Limit > 0 && opts.Limit < len(snapshots) {
		snapshots = snapshots[:opts.Limit]
	}

	return snapshots, nil
}

// GetRef reads a reference.
func (ms *MemoryStorage) GetRef(name string) (*core.Reference, error) {
	v, ok := ms.refs.Load(name)
	if !ok {
		return nil, errors.New("reference not found")
	}
	return v.(*core.Reference), nil
}

// SetRef writes a reference.
func (ms *MemoryStorage) SetRef(name string, ref *core.Reference) error {
	ms.refs.Store(name, ref)
	return nil
}

// ListRefs lists all references matching the given prefix.
func (ms *MemoryStorage) ListRefs(prefix string) ([]*core.Reference, error) {
	var refs []*core.Reference
	ms.refs.Range(func(key, value any) bool {
		name := key.(string)
		if strings.HasPrefix(name, prefix) {
			refs = append(refs, value.(*core.Reference))
		}
		return true
	})
	return refs, nil
}

// DeleteRef removes a reference.
func (ms *MemoryStorage) DeleteRef(name string) error {
	ms.refs.Delete(name)
	return nil
}

// GetIndex retrieves the staging index.
func (ms *MemoryStorage) GetIndex() (*core.Index, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.index == nil {
		return &core.Index{}, nil
	}
	return ms.index, nil
}

// SetIndex stores the staging index.
func (ms *MemoryStorage) SetIndex(index *core.Index) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.index = index
	return nil
}

// GetPreview is a noop stub (Phase 1).
func (ms *MemoryStorage) GetPreview(hash core.Hash, size int) ([]byte, error) {
	return nil, nil
}

// PutPreview is a noop stub (Phase 1).
func (ms *MemoryStorage) PutPreview(hash core.Hash, size int, data []byte) error {
	return nil
}

// GetConfig retrieves the configuration.
func (ms *MemoryStorage) GetConfig() (*core.Config, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.config == nil {
		return core.DefaultConfig(), nil
	}
	return ms.config, nil
}

// SetConfig stores the configuration.
func (ms *MemoryStorage) SetConfig(config *core.Config) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.config = config
	return nil
}
