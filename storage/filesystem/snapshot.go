package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/util/fsutil"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
)

// snapshotPath returns the filesystem path for a snapshot.
func (fs *FSStorage) snapshotPath(id core.SnapshotID) string {
	hex := id.Hash.FullString()
	return filepath.Join(fs.root, SnapshotsDir, hex[:2], hex[2:])
}

// GetSnapshot reads a snapshot from disk.
func (fs *FSStorage) GetSnapshot(ctx context.Context, id core.SnapshotID) (*core.Snapshot, error) {
	path := fs.snapshotPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("get snapshot %x: %w", id.Hash[:8], storage.ErrNotFound)
		}
		return nil, err
	}
	p := &core.SnapshotProto{}
	if err := proto.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot %x: %w", id.Hash[:8], storage.ErrCorrupted)
	}
	return snapshotFromProto(p), nil
}

// PutSnapshot writes a snapshot to disk.
func (fs *FSStorage) PutSnapshot(ctx context.Context, snapshot *core.Snapshot) error {
	// withIDHash=true: the ID is already assigned, so persist it.
	p := core.SnapshotToProto(snapshot, true)
	data, err := proto.Marshal(p)
	if err != nil {
		return err
	}
	path := fs.snapshotPath(snapshot.ID)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, data, 0644)
}

// DeleteSnapshot removes a snapshot from disk. It is idempotent:
// a missing file is not an error.
func (fs *FSStorage) DeleteSnapshot(ctx context.Context, id core.SnapshotID) error {
	path := fs.snapshotPath(id)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ListSnapshots lists all snapshots, sorted by timestamp descending,
// with optional limit/offset and branch filter.
func (fs *FSStorage) ListSnapshots(ctx context.Context, opts *storage.ListOptions) ([]*core.Snapshot, error) {
	snapDir := filepath.Join(fs.root, SnapshotsDir)
	var snapshots []*core.Snapshot

	err := filepath.WalkDir(snapDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		p := &core.SnapshotProto{}
		if err := proto.Unmarshal(data, p); err != nil {
			return fmt.Errorf("unmarshal snapshot: %w", storage.ErrCorrupted)
		}
		snapshots = append(snapshots, snapshotFromProto(p))
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return snapshots, nil
		}
		return nil, err
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp > snapshots[j].Timestamp
	})

	if opts == nil {
		return snapshots, nil
	}

	// Filter by branch if specified
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

// --- protobuf conversion helpers ---

// snapshotToProto now lives in the core package (core.SnapshotToProto) so the
// porcelain and storage layers share a single, drift-stable serialization.
// snapshotFromProto stays here because only the storage layer needs to decode
// persisted snapshots.

func snapshotFromProto(p *core.SnapshotProto) *core.Snapshot {
	if p == nil {
		return nil
	}
	s := &core.Snapshot{
		ID:        core.SnapshotID{Hash: bytesToHash(p.IdHash)},
		Message:   p.Message,
		Author:    p.Author,
		Timestamp: p.Timestamp,
		Tags:      p.Tags,
		TotalSize: p.TotalSize,
	}
	if len(p.PrevIdHash) > 0 {
		prevID := core.SnapshotID{Hash: bytesToHash(p.PrevIdHash)}
		s.PrevID = &prevID
	}
	for _, fe := range p.Files {
		f := core.FileEntry{
			Path:    fe.Path,
			Mode:    core.FileMode(fe.Mode),
			Size:    fe.Size,
			ModTime: fe.ModTime,
		}
		for _, ch := range fe.ChunkHashes {
			f.Chunks = append(f.Chunks, bytesToHash(ch))
		}
		if len(fe.FileHash) == 32 {
			// New snapshots store the file-level hash directly; use it
			// as-is to avoid recomputing from chunk hashes.
			copy(f.Hash[:], fe.FileHash)
		} else {
			// Old snapshots (no file_hash): recompute from chunk hashes
			// for backward compatibility (same method as CreateSnapshot).
			fileHasher := blake3.New()
			for _, h := range f.Chunks {
				fileHasher.Write(h[:])
			}
			copy(f.Hash[:], fileHasher.Sum(nil))
		}
		if fe.MimeType != nil {
			f.Metadata = &core.FileMetadata{MimeType: *fe.MimeType}
		}
		s.Files = append(s.Files, f)
	}
	return s
}

func bytesToHash(b []byte) core.Hash {
	var h core.Hash
	copy(h[:], b)
	return h
}

func copyBytes(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
