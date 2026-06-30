package memory

import (
	"context"
	"encoding/hex"
	"fmt"
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
func (ms *MemoryStorage) HasChunk(ctx context.Context, hash core.Hash) bool {
	_, ok := ms.chunks.Load(hash.FullString())
	return ok
}

// GetChunk retrieves a chunk.
func (ms *MemoryStorage) GetChunk(ctx context.Context, hash core.Hash) (*core.Chunk, error) {
	v, ok := ms.chunks.Load(hash.FullString())
	if !ok {
		return nil, fmt.Errorf("get chunk %s: %w", hash.FullString(), storage.ErrNotFound)
	}
	return cloneChunk(v.(*core.Chunk)), nil
}

// PutChunk stores a chunk.
func (ms *MemoryStorage) PutChunk(ctx context.Context, chunk *core.Chunk) error {
	ms.chunks.Store(chunk.Hash.FullString(), cloneChunk(chunk))
	return nil
}

// DeleteChunk removes a chunk. It is idempotent.
func (ms *MemoryStorage) DeleteChunk(ctx context.Context, hash core.Hash) error {
	ms.chunks.Delete(hash.FullString())
	return nil
}

// ListChunks returns the hashes of all stored chunks. The order of the
// returned slice is not guaranteed.
func (ms *MemoryStorage) ListChunks(ctx context.Context) ([]core.Hash, error) {
	var hashes []core.Hash
	ms.chunks.Range(func(key, value any) bool {
		b, err := hex.DecodeString(key.(string))
		if err != nil {
			return true
		}
		var h core.Hash
		copy(h[:], b)
		hashes = append(hashes, h)
		return true
	})
	return hashes, nil
}

// GetSnapshot retrieves a snapshot.
func (ms *MemoryStorage) GetSnapshot(ctx context.Context, id core.SnapshotID) (*core.Snapshot, error) {
	v, ok := ms.snapshots.Load(id.Hash.FullString())
	if !ok {
		return nil, fmt.Errorf("get snapshot %s: %w", id.Hash.FullString(), storage.ErrNotFound)
	}
	return cloneSnapshot(v.(*core.Snapshot)), nil
}

// PutSnapshot stores a snapshot.
func (ms *MemoryStorage) PutSnapshot(ctx context.Context, snapshot *core.Snapshot) error {
	ms.snapshots.Store(snapshot.ID.Hash.FullString(), cloneSnapshot(snapshot))
	return nil
}

// DeleteSnapshot removes a snapshot. It is idempotent.
func (ms *MemoryStorage) DeleteSnapshot(ctx context.Context, id core.SnapshotID) error {
	ms.snapshots.Delete(id.Hash.FullString())
	return nil
}

// ListSnapshots lists all snapshots, sorted by timestamp descending,
// with optional limit/offset and branch filter.
func (ms *MemoryStorage) ListSnapshots(ctx context.Context, opts *storage.ListOptions) ([]*core.Snapshot, error) {
	var snapshots []*core.Snapshot
	ms.snapshots.Range(func(key, value any) bool {
		snapshots = append(snapshots, cloneSnapshot(value.(*core.Snapshot)))
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

// maxSymRefDepth bounds the number of symbolic-reference hops GetRef will
// follow before giving up. It guards against malformed or malicious
// self-referential symrefs (e.g. HEAD -> HEAD) that would otherwise cause
// unbounded recursion.
const maxSymRefDepth = 8

// GetRef reads a reference. If the reference is a symbolic reference,
// Target is resolved by recursively reading the referenced ref.
func (ms *MemoryStorage) GetRef(ctx context.Context, name string) (*core.Reference, error) {
	return ms.getRefRecursive(ctx, name, 0)
}

func (ms *MemoryStorage) getRefRecursive(ctx context.Context, name string, depth int) (*core.Reference, error) {
	if depth > maxSymRefDepth {
		return nil, fmt.Errorf("symref recursion limit exceeded at %q: %w", name, storage.ErrInvalidRef)
	}
	v, ok := ms.refs.Load(name)
	if !ok {
		return nil, fmt.Errorf("get ref %q: %w", name, storage.ErrNotFound)
	}
	ref := v.(*core.Reference)
	if ref.SymRef != "" {
		target, err := ms.getRefRecursive(ctx, ref.SymRef, depth+1)
		if err != nil {
			return nil, err
		}
		resolved := cloneReference(ref)
		resolved.Target = target.Target
		return resolved, nil
	}
	return cloneReference(ref), nil
}

// SetRef writes a reference.
func (ms *MemoryStorage) SetRef(ctx context.Context, name string, ref *core.Reference) error {
	ms.refs.Store(name, cloneReference(ref))
	return nil
}

// ListRefs lists all references matching the given prefix.
func (ms *MemoryStorage) ListRefs(ctx context.Context, prefix string) ([]*core.Reference, error) {
	var refs []*core.Reference
	ms.refs.Range(func(key, value any) bool {
		name := key.(string)
		if strings.HasPrefix(name, prefix) {
			refs = append(refs, cloneReference(value.(*core.Reference)))
		}
		return true
	})
	return refs, nil
}

// DeleteRef removes a reference.
func (ms *MemoryStorage) DeleteRef(ctx context.Context, name string) error {
	ms.refs.Delete(name)
	return nil
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
	return nil, nil
}

// PutPreview is a noop stub (Phase 1).
func (ms *MemoryStorage) PutPreview(ctx context.Context, hash core.Hash, size int, data []byte) error {
	return nil
}

// GetConfig retrieves the configuration.
func (ms *MemoryStorage) GetConfig(ctx context.Context) (*core.Config, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.config == nil {
		return core.DefaultConfig(), nil
	}
	return cloneConfig(ms.config), nil
}

// SetConfig stores the configuration.
func (ms *MemoryStorage) SetConfig(ctx context.Context, config *core.Config) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.config = cloneConfig(config)
	return nil
}

// Close releases any resources held by the memory storage. It is a no-op.
func (ms *MemoryStorage) Close() error {
	return nil
}

// cloneChunk returns a deep copy of a Chunk.
func cloneChunk(c *core.Chunk) *core.Chunk {
	if c == nil {
		return nil
	}
	clone := &core.Chunk{
		Hash:  c.Hash,
		Size:  c.Size,
		Flags: c.Flags,
	}
	if c.Data != nil {
		clone.Data = make([]byte, len(c.Data))
		copy(clone.Data, c.Data)
	}
	return clone
}

// cloneFileEntry returns a deep copy of a FileEntry.
func cloneFileEntry(f core.FileEntry) core.FileEntry {
	clone := f
	if f.Chunks != nil {
		clone.Chunks = make([]core.Hash, len(f.Chunks))
		copy(clone.Chunks, f.Chunks)
	}
	if f.Metadata != nil {
		m := *f.Metadata
		if f.Metadata.Extra != nil {
			m.Extra = make(map[string]string, len(f.Metadata.Extra))
			for k, v := range f.Metadata.Extra {
				m.Extra[k] = v
			}
		}
		clone.Metadata = &m
	}
	return clone
}

// cloneSnapshot returns a deep copy of a Snapshot.
func cloneSnapshot(s *core.Snapshot) *core.Snapshot {
	if s == nil {
		return nil
	}
	clone := &core.Snapshot{
		ID:        s.ID,
		Message:   s.Message,
		Author:    s.Author,
		Timestamp: s.Timestamp,
		TotalSize: s.TotalSize,
	}
	if s.PrevID != nil {
		prev := *s.PrevID
		clone.PrevID = &prev
	}
	if s.Files != nil {
		clone.Files = make([]core.FileEntry, len(s.Files))
		for i, f := range s.Files {
			clone.Files[i] = cloneFileEntry(f)
		}
	}
	if s.Tags != nil {
		clone.Tags = make([]string, len(s.Tags))
		copy(clone.Tags, s.Tags)
	}
	return clone
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
