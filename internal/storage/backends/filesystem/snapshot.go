package filesystem

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/util/fsutil"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
)

// snapshotPath returns the filesystem path for a snapshot.
func (fs *FSStorage) snapshotPath(id core.SnapshotID) string {
	hexStr := id.Hash.FullString()
	return filepath.Join(fs.root, SnapshotsDir, hexStr[:2], hexStr[2:])
}

// manifestPath returns the filesystem path for a snapshot manifest.
func (fs *FSStorage) manifestPath(id core.SnapshotID) string {
	hexStr := id.Hash.FullString()
	return filepath.Join(fs.root, ManifestsDir, hexStr[:2], hexStr[2:])
}

// GetSnapshot reads a snapshot from disk and verifies its integrity.
func (fs *FSStorage) GetSnapshot(ctx context.Context, id core.SnapshotID) (*core.Snapshot, error) {
	path := fs.snapshotPath(id)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("get snapshot %x: %w", id.Hash[:8], storage.ErrNotFound)
		}
		return nil, fmt.Errorf("open snapshot %x: %w", id.Hash[:8], err)
	}
	defer f.Close()
	// google.golang.org/protobuf (v1.36.x) does not expose a reader-based
	// streaming decoder (proto.NewDecoder was removed in the
	// github.com/golang/protobuf v1.5.0 shim). We stream the file through
	// os.Open + io.ReadAll and release the raw buffer before the integrity
	// check so peak memory is the decoded message plus the re-marshaled
	// bytes, not raw+message+re-marshaled.
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read snapshot %x: %w", id.Hash[:8], err)
	}
	p := &core.SnapshotProto{}
	if err := proto.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot %x: %w", id.Hash[:8], storage.ErrCorrupted)
	}
	data = nil // allow GC of raw file bytes before the integrity re-marshal
	// Verify integrity: the snapshot ID is the BLAKE3 hash of the marshaled
	// proto with IdHash omitted (as computed in porcelain.CreateSnapshot).
	// Clear IdHash, re-marshal, and compare the hash to the requested ID.
	idHash := p.IdHash
	p.IdHash = nil
	recomputed, err := proto.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("re-marshal snapshot %x: %w", id.Hash[:8], storage.ErrCorrupted)
	}
	if core.Hash(blake3.Sum256(recomputed)) != id.Hash {
		return nil, fmt.Errorf("snapshot %x integrity check failed: %w", id.Hash[:8], storage.ErrCorrupted)
	}
	p.IdHash = idHash
	return snapshotFromProto(p), nil
}

// PutSnapshot writes a snapshot and its lightweight manifest to disk.
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
	if err := fsutil.WriteFileAtomic(path, data, 0644); err != nil {
		return err
	}
	return fs.writeManifest(snapshot)
}

// writeManifest writes the lightweight manifest for a snapshot so that
// ListSnapshots can enumerate metadata without deserializing the full file list.
func (fs *FSStorage) writeManifest(snapshot *core.Snapshot) error {
	m := core.SnapshotToManifest(snapshot)
	data, err := proto.Marshal(m)
	if err != nil {
		return err
	}
	path := fs.manifestPath(snapshot.ID)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, data, 0644)
}

// DeleteSnapshot removes a snapshot and its manifest from disk. It is
// idempotent: a missing file is not an error.
func (fs *FSStorage) DeleteSnapshot(ctx context.Context, id core.SnapshotID) error {
	path := fs.snapshotPath(id)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	// Best-effort manifest cleanup; a missing manifest is not an error.
	mPath := fs.manifestPath(id)
	if err := os.Remove(mPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ListSnapshots lists all snapshots via lightweight manifests, sorted by
// timestamp descending, with optional limit/offset pagination. Returns
// snapshot summaries without file lists — call GetSnapshot for full details.
//
// Snapshots without a manifest (e.g. created before manifests were introduced)
// fall back to reading the full snapshot file and backfilling a manifest so
// subsequent listings are fast.
func (fs *FSStorage) ListSnapshots(ctx context.Context, opts *storage.ListOptions) ([]*core.SnapshotSummary, error) {
	// Bail out early if the caller has already cancelled, before we start
	// walking the manifests directory.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	manifests := make(map[core.SnapshotID]*core.SnapshotManifest)

	// Phase 1: read all manifests (lightweight, typically < 1KB each).
	manDir := filepath.Join(fs.root, ManifestsDir)
	err := filepath.WalkDir(manDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		m := &core.SnapshotManifest{}
		if err := proto.Unmarshal(data, m); err != nil {
			slog.Warn("skipping corrupted manifest", "path", path)
			return nil
		}
		manifests[manifestID(m)] = m
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("walk manifests: %w", err)
	}

	// Phase 2: find snapshots without manifests (legacy/corrupted), read the
	// full snapshot, and backfill a manifest so subsequent listings are fast.
	snapDir := filepath.Join(fs.root, SnapshotsDir)
	err = filepath.WalkDir(snapDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		id, ok := snapshotIDFromPath(path)
		if !ok {
			return nil
		}
		if _, exists := manifests[id]; exists {
			return nil
		}
		// Manifest missing: fall back to reading the full snapshot.
		data, rErr := os.ReadFile(path)
		if rErr != nil {
			slog.Warn("cannot read snapshot", "path", path, "error", rErr)
			return nil
		}
		p := &core.SnapshotProto{}
		if uErr := proto.Unmarshal(data, p); uErr != nil {
			slog.Warn("skipping corrupted snapshot", "path", path)
			return nil
		}
		m := manifestFromProto(p)
		manifests[id] = m
		// Backfill the manifest file (best-effort, errors are non-fatal).
		fs.backfillManifest(id, m)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("walk snapshots for manifest backfill: %w", err)
	}

	// Phase 3: convert manifests to summaries and sort.
	summaries := make([]*core.SnapshotSummary, 0, len(manifests))
	for _, m := range manifests {
		summaries = append(summaries, core.ManifestToSummary(m))
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Timestamp > summaries[j].Timestamp
	})

	if opts == nil {
		return summaries, nil
	}
	if opts.Offset > 0 {
		if opts.Offset >= len(summaries) {
			return nil, nil
		}
		summaries = summaries[opts.Offset:]
	}
	if opts.Limit > 0 && opts.Limit < len(summaries) {
		summaries = summaries[:opts.Limit]
	}
	return summaries, nil
}

// backfillManifest writes a manifest file for a snapshot that was missing one.
// Errors are non-fatal — manifest backfilling is best-effort so that a listing
// is never blocked by a write failure.
func (fs *FSStorage) backfillManifest(id core.SnapshotID, m *core.SnapshotManifest) {
	data, err := proto.Marshal(m)
	if err != nil {
		return
	}
	path := fs.manifestPath(id)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	_ = fsutil.WriteFileAtomic(path, data, 0644)
}

// snapshotIDFromPath extracts the SnapshotID from a snapshot file path.
// Path layout: <root>/snapshots/<hex[:2]>/<hex[2:]>.
func snapshotIDFromPath(path string) (core.SnapshotID, bool) {
	name := filepath.Base(path)
	parent := filepath.Base(filepath.Dir(path))
	hexStr := parent + name
	if len(hexStr) != 64 {
		return core.SnapshotID{}, false
	}
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return core.SnapshotID{}, false
	}
	var hash core.Hash
	copy(hash[:], b)
	return core.SnapshotID{Hash: hash}, true
}

// manifestFromProto extracts a manifest from a full SnapshotProto without
// building the FileEntry list, used in the fallback path when a manifest is
// missing and the full snapshot must be read.
func manifestFromProto(p *core.SnapshotProto) *core.SnapshotManifest {
	if p == nil {
		return nil
	}
	id := make([]byte, core.HashSize)
	copy(id, p.IdHash)
	m := &core.SnapshotManifest{
		Id:           id,
		Message:      p.Message,
		Author:       p.Author,
		Timestamp:    p.Timestamp,
		Tags:         p.Tags,
		TotalSize:    p.TotalSize,
		FilesChanged: int32(len(p.Files)),
	}
	if len(p.PrevIdHash) > 0 {
		prev := make([]byte, core.HashSize)
		copy(prev, p.PrevIdHash)
		m.PrevId = prev
	}
	return m
}

func manifestID(m *core.SnapshotManifest) core.SnapshotID {
	var id core.SnapshotID
	copy(id.Hash[:], m.Id)
	return id
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
			// Old snapshots without FileHash: leave as zero. The hash will be
			// recomputed on the next save. A zero hash causes diff to conservatively
			// report the file as modified, which is safer than a wrong hash.
			// (Previously this computed blake3 of chunk hashes, which diverged from
			// CreateSnapshot's blake3 of file content.)
		}
		if fe.MimeType != nil || len(fe.Extra) > 0 {
			f.Metadata = &core.FileMetadata{}
			if fe.MimeType != nil {
				f.Metadata.MIMEType = *fe.MimeType
			}
			if len(fe.Extra) > 0 {
				f.Metadata.Extra = make(map[string]string, len(fe.Extra))
				for k, v := range fe.Extra {
					f.Metadata.Extra[k] = v
				}
			}
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
