package filesystem

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/util/fsutil"
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
		return nil, fmt.Errorf("open snapshot %x: %w", id.Hash[:8], mapOSError(err))
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
	snap, err := snapshotFromProto(p)
	if err != nil {
		return nil, fmt.Errorf("decode snapshot %x: %w", id.Hash[:8], err)
	}
	return snap, nil
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
	if err := os.MkdirAll(dir, fsutil.DefaultDirPerm); err != nil {
		return err
	}
	if err := fsutil.WriteFileAtomic(path, data, fsutil.DefaultFilePerm); err != nil {
		return fmt.Errorf("write snapshot %x: %w", snapshot.ID.Hash[:8], mapOSError(err))
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
	if err := os.MkdirAll(dir, fsutil.DefaultDirPerm); err != nil {
		return err
	}
	if err := fsutil.WriteFileAtomic(path, data, fsutil.DefaultFilePerm); err != nil {
		return fmt.Errorf("write manifest %x: %w", snapshot.ID.Hash[:8], mapOSError(err))
	}
	return nil
}

// DeleteSnapshot removes a snapshot and its manifest from disk. It is
// idempotent: a missing file is not an error.
func (fs *FSStorage) DeleteSnapshot(ctx context.Context, id core.SnapshotID) error {
	path := fs.snapshotPath(id)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete snapshot %x: %w", id.Hash[:8], mapOSError(err))
	}
	// Best-effort manifest cleanup; a missing manifest is not an error.
	mPath := fs.manifestPath(id)
	if err := os.Remove(mPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete manifest %x: %w", id.Hash[:8], mapOSError(err))
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

	return storage.ApplySummaryPagination(summaries, opts), nil
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
	if err := os.MkdirAll(dir, fsutil.DefaultDirPerm); err != nil {
		return
	}
	_ = fsutil.WriteFileAtomic(path, data, fsutil.DefaultFilePerm)
}
