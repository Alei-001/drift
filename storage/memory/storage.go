package memory

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/storage/refname"
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

// GetSnapshot retrieves a snapshot.
func (ms *MemoryStorage) GetSnapshot(ctx context.Context, id core.SnapshotID) (*core.Snapshot, error) {
	v, ok := ms.snapshots[id.Hash.FullString()]
	if !ok {
		return nil, fmt.Errorf("get snapshot %s: %w", id.Hash.FullString(), storage.ErrNotFound)
	}
	return storage.CloneSnapshot(v), nil
}

// PutSnapshot stores a snapshot and caches its lightweight manifest.
func (ms *MemoryStorage) PutSnapshot(ctx context.Context, snapshot *core.Snapshot) error {
	key := snapshot.ID.Hash.FullString()
	ms.snapshots[key] = storage.CloneSnapshot(snapshot)
	ms.manifests[key] = cloneManifest(core.SnapshotToManifest(snapshot))
	return nil
}

// DeleteSnapshot removes a snapshot and its manifest. It is idempotent.
func (ms *MemoryStorage) DeleteSnapshot(ctx context.Context, id core.SnapshotID) error {
	key := id.Hash.FullString()
	delete(ms.snapshots, key)
	delete(ms.manifests, key)
	return nil
}

// ListSnapshots lists all snapshots via lightweight manifests, sorted by
// timestamp descending, with optional limit/offset pagination. The returned
// snapshots have nil Files because manifests carry only metadata.
func (ms *MemoryStorage) ListSnapshots(ctx context.Context, opts *storage.ListOptions) ([]*core.Snapshot, error) {
	var snapshots []*core.Snapshot
	for _, m := range ms.manifests {
		snapshots = append(snapshots, core.ManifestToSnapshot(m))
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp > snapshots[j].Timestamp
	})

	if opts == nil {
		return snapshots, nil
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

// GetRef reads a reference. If the reference is a symbolic reference,
// Target is resolved by recursively reading the referenced ref.
func (ms *MemoryStorage) GetRef(ctx context.Context, name string) (*core.Reference, error) {
	return ms.getRefRecursive(ctx, name, 0)
}

func (ms *MemoryStorage) getRefRecursive(ctx context.Context, name string, depth int) (*core.Reference, error) {
	if depth > storage.MaxSymRefDepth {
		return nil, fmt.Errorf("symref recursion limit exceeded at %q: %w", name, storage.ErrInvalidRef)
	}
	if err := refname.Validate(name); err != nil {
		return nil, fmt.Errorf("validate ref %q: %w", name, err)
	}
	ref, ok := ms.refs[name]
	if !ok {
		return nil, fmt.Errorf("get ref %q: %w", name, storage.ErrNotFound)
	}
	if ref.SymRef != "" {
		target, err := ms.getRefRecursive(ctx, ref.SymRef, depth+1)
		if err != nil {
			return nil, fmt.Errorf("resolve symref %q: %w", ref.SymRef, err)
		}
		resolved := cloneReference(ref)
		resolved.Name = name
		resolved.Target = target.Target
		// Derive Type from name, matching the filesystem backend's behavior
		// (the stored Type field is ignored so both backends agree).
		resolved.Type = refTypeFromName(name)
		return resolved, nil
	}
	clone := cloneReference(ref)
	clone.Name = name
	clone.Type = refTypeFromName(name)
	return clone, nil
}

// SetRef writes a reference.
func (ms *MemoryStorage) SetRef(ctx context.Context, name string, ref *core.Reference) error {
	if err := refname.Validate(name); err != nil {
		return fmt.Errorf("validate ref %q: %w", name, err)
	}
	clone := cloneReference(ref)
	if clone.SymRef != "" {
		// Normalize SymRef by stripping any "refs/" prefix so subsequent
		// GetRef lookups succeed regardless of how the caller wrote it.
		// This mirrors the filesystem backend's on-disk format.
		clone.SymRef = strings.TrimPrefix(clone.SymRef, "refs/")
		if err := refname.Validate(clone.SymRef); err != nil {
			return fmt.Errorf("validate symref %q: %w", clone.SymRef, err)
		}
	}
	ms.refs[name] = clone
	return nil
}

// refTypeFromName derives the RefType from the ref name, matching the
// filesystem backend's refType() logic so both backends return the same
// Type for the same name.
func refTypeFromName(name string) core.RefType {
	if name == "HEAD" {
		return core.RefTypeHead
	}
	if strings.HasPrefix(name, "heads/") {
		return core.RefTypeBranch
	}
	if strings.HasPrefix(name, "tags/") {
		return core.RefTypeTag
	}
	return core.RefTypeBranch
}

// ListRefs lists all references matching the given prefix.
// HEAD is excluded to match the filesystem backend, which only walks the
// refs/ directory (HEAD lives at the repository root, outside refs/).
// Each ref is resolved via GetRef so symrefs have their Target populated
// and Type derived from the name, matching the filesystem backend.
//
// Only ErrNotFound errors from GetRef are skipped (e.g. dangling symref);
// other errors are propagated so callers can distinguish I/O failures from
// missing refs.
func (ms *MemoryStorage) ListRefs(ctx context.Context, prefix string) ([]*core.Reference, error) {
	var refs []*core.Reference
	for name := range ms.refs {
		if name == "HEAD" {
			continue
		}
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		ref, err := ms.GetRef(ctx, name)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			return nil, fmt.Errorf("list refs: resolve %q: %w", name, err)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// DeleteRef removes a reference.
func (ms *MemoryStorage) DeleteRef(ctx context.Context, name string) error {
	if err := refname.Validate(name); err != nil {
		return fmt.Errorf("validate ref %q: %w", name, err)
	}
	if name == "HEAD" {
		return fmt.Errorf("cannot delete HEAD: %w", storage.ErrInvalidRef)
	}
	delete(ms.refs, name)
	return nil
}

// GetIndex retrieves the staging index.
func (ms *MemoryStorage) GetIndex(ctx context.Context) (*core.Index, error) {
	if ms.index == nil {
		return &core.Index{}, nil
	}
	return cloneIndex(ms.index), nil
}

// SetIndex stores the staging index.
func (ms *MemoryStorage) SetIndex(ctx context.Context, index *core.Index) error {
	ms.index = cloneIndex(index)
	return nil
}

// GetPreview is a noop stub (Phase 1).
func (ms *MemoryStorage) GetPreview(ctx context.Context, hash core.Hash, size int) ([]byte, error) {
	return nil, fmt.Errorf("get preview %s: %w", hash.FullString(), storage.ErrNotFound)
}

// PutPreview is a noop stub (Phase 1).
func (ms *MemoryStorage) PutPreview(ctx context.Context, hash core.Hash, size int, data []byte) error {
	return nil
}

// Clamp chunk sizes to reasonable ranges to prevent OOM. Values are shared
// from the storage package so both backends use the same limits.
func (ms *MemoryStorage) GetConfig(ctx context.Context) (*core.Config, error) {
	if ms.config == nil {
		return core.DefaultConfig(), nil
	}
	// Clone before returning so callers cannot mutate stored state, and
	// apply shared normalization so tests that SetConfig a partial config
	// observe the same field invariants as the filesystem backend.
	clone := cloneConfig(ms.config)
	clone.Core.Normalize()
	// Apply upper-bound clamps to match the filesystem backend.
	if clone.Core.ChunkMinSize > storage.MaxChunkMinSize {
		clone.Core.ChunkMinSize = storage.MaxChunkMinSize
	}
	if clone.Core.ChunkAvgSize > storage.MaxChunkAvgSize {
		clone.Core.ChunkAvgSize = storage.MaxChunkAvgSize
	}
	if clone.Core.ChunkMaxSize > storage.MaxChunkMaxSize {
		clone.Core.ChunkMaxSize = storage.MaxChunkMaxSize
	}
	return clone, nil
}

// SetConfig stores the configuration.
func (ms *MemoryStorage) SetConfig(ctx context.Context, config *core.Config) error {
	ms.config = cloneConfig(config)
	return nil
}

// Close releases any resources held by the memory storage. It is a no-op.
func (ms *MemoryStorage) Close() error {
	return nil
}

// cloneManifest returns a deep copy of a SnapshotManifest.
func cloneManifest(m *core.SnapshotManifest) *core.SnapshotManifest {
	if m == nil {
		return nil
	}
	clone := *m
	if m.Id != nil {
		clone.Id = make([]byte, len(m.Id))
		copy(clone.Id, m.Id)
	}
	if m.PrevId != nil {
		clone.PrevId = make([]byte, len(m.PrevId))
		copy(clone.PrevId, m.PrevId)
	}
	if m.Tags != nil {
		clone.Tags = make([]string, len(m.Tags))
		copy(clone.Tags, m.Tags)
	}
	return &clone
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
