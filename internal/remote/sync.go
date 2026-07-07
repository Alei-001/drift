package remote

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/backends/filesystem"
	"github.com/klauspost/compress/zstd"
	"google.golang.org/protobuf/proto"
)

// chunkHeaderSize and chunkFlagCompressed mirror the constants in
// filesystem/chunk.go. They are duplicated here because sync.go needs to
// re-encode chunks for upload (push) and decode them on download (pull),
// without depending on the filesystem package's unexported constants.
// The remote wire format MUST match the local on-disk format.
const (
	chunkHeaderSize     = 1
	chunkFlagCompressed byte = 0x01
)

// SyncStats reports the outcome of a push or pull operation.
type SyncStats struct {
	SnapshotsUploaded   int
	SnapshotsSkipped    int
	ManifestsUploaded   int
	ChunksUploaded      int
	ChunksSkipped       int
	RefsUpdated         int
	RefsDiverged        int // pull only: refs saved as <name>.remote
	IndexRebuilt        bool
	BranchTipChanged    string // branch name whose tip advanced ("" if none)
}

// Push uploads local objects (snapshots, manifests, chunks, refs) to the
// remote. Objects already present on the remote are skipped. Refs that
// diverge (same name, different target) cause an error for that ref — the
// user must pull first. HEAD and config are NOT synced (see design doc §6.1).
//
// If branch is non-empty, only that branch's snapshot chain and its chunks
// are synced, plus the branch ref itself.
func Push(ctx context.Context, store storage.Storer, rfs RemoteFS, branch string) (*SyncStats, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stats := &SyncStats{}

	snapHashes, chunkHashes, refs, err := collectPushScope(ctx, store, branch)
	if err != nil {
		return nil, fmt.Errorf("collect push scope: %w", err)
	}

	// Upload snapshots + manifests.
	for _, id := range snapHashes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		snapPath := snapshotRemotePath(id)
		if _, err := rfs.Stat(snapPath); err == nil {
			stats.SnapshotsSkipped++
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("stat remote snapshot %s: %w", id.Hash.String(), err)
		}
		if err := pushSnapshot(ctx, store, rfs, id); err != nil {
			return nil, fmt.Errorf("push snapshot %s: %w", id.Hash.String(), err)
		}
		stats.SnapshotsUploaded++
		// Manifest is derived from the snapshot, uploaded alongside.
		if err := pushManifest(ctx, store, rfs, id); err != nil {
			return nil, fmt.Errorf("push manifest %s: %w", id.Hash.String(), err)
		}
		stats.ManifestsUploaded++
	}

	// Upload chunks.
	for _, ch := range chunkHashes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chPath := chunkRemotePath(ch)
		if _, err := rfs.Stat(chPath); err == nil {
			stats.ChunksSkipped++
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("stat remote chunk %s: %w", ch.String(), err)
		}
		if err := pushChunk(ctx, store, rfs, ch); err != nil {
			return nil, fmt.Errorf("push chunk %s: %w", ch.String(), err)
		}
		stats.ChunksUploaded++
	}

	// Upload refs (only those whose target snapshot exists on the remote).
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := pushRef(ctx, rfs, ref); err != nil {
			if errors.Is(err, errRefDiverged) {
				return nil, fmt.Errorf("ref %q diverged: pull first: %w", ref.Name, err)
			}
			return nil, fmt.Errorf("push ref %q: %w", ref.Name, err)
		}
		stats.RefsUpdated++
	}

	return stats, nil
}

// Pull downloads remote objects to local. Objects already present locally
// are skipped. Diverged refs (same name, different target) are saved as
// <name>.remote locally. HEAD and config are NOT synced. After pulling, if
// the current branch tip changed, the local index is rebuilt.
//
// If branch is non-empty, only that branch's snapshot chain and its chunks
// are synced, plus the branch ref itself.
func Pull(ctx context.Context, store storage.Storer, rfs RemoteFS, branch string) (*SyncStats, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stats := &SyncStats{}

	// Determine old tip (before pull) to detect tip change.
	oldTip, _ := currentBranchTip(ctx, store)

	snapHashes, chunkHashes, refs, err := collectPullScope(ctx, rfs, branch)
	if err != nil {
		return nil, fmt.Errorf("collect pull scope: %w", err)
	}

	// Download snapshots.
	for _, id := range snapHashes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Check existence via GetSnapshot + ErrNotFound (no HasSnapshot in the interface).
		if _, err := store.GetSnapshot(ctx, id); err == nil {
			stats.SnapshotsSkipped++
			continue
		} else if !errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("check local snapshot %s: %w", id.Hash.String(), err)
		}
		if err := pullSnapshot(ctx, store, rfs, id); err != nil {
			return nil, fmt.Errorf("pull snapshot %s: %w", id.Hash.String(), err)
		}
		stats.SnapshotsUploaded++
	}

	// Download chunks.
	for _, ch := range chunkHashes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		has, err := store.HasChunk(ctx, ch)
		if err != nil {
			return nil, fmt.Errorf("check local chunk %s: %w", ch.String(), err)
		}
		if has {
			stats.ChunksSkipped++
			continue
		}
		if err := pullChunk(ctx, store, rfs, ch); err != nil {
			return nil, fmt.Errorf("pull chunk %s: %w", ch.String(), err)
		}
		stats.ChunksUploaded++
	}

	// Merge refs (append-only, never overwrite).
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		updated, diverged, err := pullRef(ctx, store, rfs, ref)
		if err != nil {
			return nil, fmt.Errorf("pull ref %q: %w", ref.Name, err)
		}
		if updated {
			stats.RefsUpdated++
		}
		if diverged {
			stats.RefsDiverged++
		}
	}

	// Rebuild index if current branch tip changed.
	newTip, err := currentBranchTip(ctx, store)
	if err == nil && newTip.Hash != oldTip.Hash && !newTip.Hash.IsZero() {
		if err := rebuildIndex(ctx, store, newTip); err != nil {
			return nil, fmt.Errorf("rebuild index: %w", err)
		}
		stats.IndexRebuilt = true
		branchName, _ := currentBranchName(ctx, store)
		stats.BranchTipChanged = branchName
	}

	return stats, nil
}

// --- path helpers ---

func snapshotRemotePath(id core.SnapshotID) string {
	h := id.Hash.FullString()
	return path.Join(filesystem.SnapshotsDir, h[:2], h[2:])
}

func manifestRemotePath(id core.SnapshotID) string {
	h := id.Hash.FullString()
	return path.Join(filesystem.ManifestsDir, h[:2], h[2:])
}

func chunkRemotePath(h core.Hash) string {
	hex := h.FullString()
	return path.Join(filesystem.ChunksDir, hex[:2], hex[2:])
}

// --- push helpers ---

func pushSnapshot(ctx context.Context, store storage.Storer, rfs RemoteFS, id core.SnapshotID) error {
	snap, err := store.GetSnapshot(ctx, id)
	if err != nil {
		return fmt.Errorf("get snapshot: %w", err)
	}
	p := core.SnapshotToProto(snap, true)
	data, err := proto.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	return rfs.Write(snapshotRemotePath(id), bytes.NewReader(data))
}

func pushManifest(ctx context.Context, store storage.Storer, rfs RemoteFS, id core.SnapshotID) error {
	snap, err := store.GetSnapshot(ctx, id)
	if err != nil {
		return fmt.Errorf("get snapshot for manifest: %w", err)
	}
	m := core.SnapshotToManifest(snap)
	data, err := proto.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return rfs.Write(manifestRemotePath(id), bytes.NewReader(data))
}

func pushChunk(ctx context.Context, store storage.Storer, rfs RemoteFS, h core.Hash) error {
	chunk, err := store.GetChunk(ctx, h)
	if err != nil {
		return fmt.Errorf("get chunk: %w", err)
	}
	// Re-encode using the chunk's stored flags, matching the local on-disk
	// format: 1-byte header + (optionally compressed) payload.
	var payload []byte
	var header byte
	if chunk.Flags&core.ChunkFlagCompressed != 0 {
		enc, err := zstdEncoder()
		if err != nil {
			return fmt.Errorf("zstd encoder: %w", err)
		}
		compressed := enc.EncodeAll(chunk.Data, nil)
		header = chunkFlagCompressed
		payload = make([]byte, 0, chunkHeaderSize+len(compressed))
		payload = append(payload, header)
		payload = append(payload, compressed...)
	} else {
		header = 0
		payload = make([]byte, 0, chunkHeaderSize+len(chunk.Data))
		payload = append(payload, header)
		payload = append(payload, chunk.Data...)
	}
	return rfs.Write(chunkRemotePath(h), bytes.NewReader(payload))
}

var errRefDiverged = errors.New("ref diverged")

func pushRef(ctx context.Context, rfs RemoteFS, ref *core.Reference) error {
	// Only upload ref if its target snapshot exists on the remote.
	snapPath := snapshotRemotePath(core.SnapshotID{Hash: ref.Target})
	if _, err := rfs.Stat(snapPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // target snapshot not on remote yet, skip
		}
		return fmt.Errorf("stat remote snapshot for ref: %w", err)
	}
	refPath := refRemotePath(ref.Name)
	existing, err := rfs.Read(refPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read existing remote ref: %w", err)
		}
		// No existing ref, write it.
		return rfs.Write(refPath, strings.NewReader(ref.Target.FullString()+"\n"))
	}
	defer existing.Close()
	existingBytes, _ := io.ReadAll(existing)
	existingHash := strings.TrimSpace(string(existingBytes))
	if existingHash == ref.Target.FullString() {
		return nil // same, skip
	}
	return errRefDiverged
}

// --- pull helpers ---

func pullSnapshot(ctx context.Context, store storage.Storer, rfs RemoteFS, id core.SnapshotID) error {
	rc, err := rfs.Read(snapshotRemotePath(id))
	if err != nil {
		return fmt.Errorf("read remote snapshot: %w", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("read snapshot bytes: %w", err)
	}
	p := &core.SnapshotProto{}
	if err := proto.Unmarshal(data, p); err != nil {
		return fmt.Errorf("unmarshal snapshot: %w", err)
	}
	snap := core.SnapshotFromProto(p)
	if err := store.PutSnapshot(ctx, snap); err != nil {
		return fmt.Errorf("put snapshot: %w", err)
	}
	return nil
}

func pullChunk(ctx context.Context, store storage.Storer, rfs RemoteFS, h core.Hash) error {
	rc, err := rfs.Read(chunkRemotePath(h))
	if err != nil {
		return fmt.Errorf("read remote chunk: %w", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("read chunk bytes: %w", err)
	}
	if len(data) < chunkHeaderSize {
		return fmt.Errorf("chunk too short: %w", storage.ErrCorrupted)
	}
	header := data[0]
	payload := data[chunkHeaderSize:]
	var chunkData []byte
	var flags core.ChunkFlag
	if header&chunkFlagCompressed != 0 {
		dec, err := zstdDecoder()
		if err != nil {
			return fmt.Errorf("zstd decoder: %w", err)
		}
		decoded, err := dec.DecodeAll(payload, nil)
		if err != nil {
			return fmt.Errorf("decode chunk: %w", storage.ErrCorrupted)
		}
		chunkData = decoded
		flags = core.ChunkFlagCompressed
	} else {
		chunkData = payload
		flags = core.ChunkFlagNone
	}
	chunk := &core.Chunk{
		Hash:  h,
		Size:  uint32(len(chunkData)),
		Data:  chunkData,
		Flags: flags,
	}
	return store.PutChunk(ctx, chunk)
}

func pullRef(ctx context.Context, store storage.Storer, rfs RemoteFS, ref *core.Reference) (updated bool, diverged bool, err error) {
	localRef, err := store.GetRef(ctx, ref.Name)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return false, false, fmt.Errorf("get local ref: %w", err)
		}
		// Local ref doesn't exist, write it directly.
		if err := store.SetRef(ctx, ref.Name, ref); err != nil {
			return false, false, fmt.Errorf("set ref: %w", err)
		}
		return true, false, nil
	}
	if localRef.Target == ref.Target {
		return false, false, nil // same, skip
	}
	// Diverged: save remote version as <name>.remote.
	remoteName := ref.Name + ".remote"
	remoteRef := &core.Reference{
		Name:   remoteName,
		Type:   ref.Type,
		Target: ref.Target,
	}
	if err := store.SetRef(ctx, remoteName, remoteRef); err != nil {
		return false, false, fmt.Errorf("set diverged ref: %w", err)
	}
	return false, true, nil
}

// --- scope collection ---

func collectPushScope(ctx context.Context, store storage.Storer, branch string) ([]core.SnapshotID, []core.Hash, []*core.Reference, error) {
	var snapIDs []core.SnapshotID
	var chunkHashes []core.Hash
	var refs []*core.Reference

	if branch != "" {
		// Branch-scoped: walk the branch's PrevID chain.
		refName := "heads/" + branch
		ref, err := store.GetRef(ctx, refName)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("get branch ref: %w", err)
		}
		refs = append(refs, ref)
		ids, chunks, err := walkSnapshotChain(ctx, store, core.SnapshotID{Hash: ref.Target})
		if err != nil {
			return nil, nil, nil, err
		}
		snapIDs = ids
		chunkHashes = chunks
	} else {
		// Full repo: list all snapshots, collect all refs.
		summaries, err := store.ListSnapshots(ctx, nil)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("list snapshots: %w", err)
		}
		seenChunks := make(map[core.Hash]bool)
		for _, s := range summaries {
			if err := ctx.Err(); err != nil {
				return nil, nil, nil, err
			}
			id := core.SnapshotID{Hash: s.ID.Hash}
			snapIDs = append(snapIDs, id)
			snap, err := store.GetSnapshot(ctx, id)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("get snapshot %s: %w", id.Hash.String(), err)
			}
			for _, f := range snap.Files {
				for _, ch := range f.Chunks {
					if !seenChunks[ch] {
						seenChunks[ch] = true
						chunkHashes = append(chunkHashes, ch)
					}
				}
			}
		}
		allRefs, err := store.ListRefs(ctx, "")
		if err != nil {
			return nil, nil, nil, fmt.Errorf("list refs: %w", err)
		}
		for _, r := range allRefs {
			if r.Name == "HEAD" {
				continue
			}
			refs = append(refs, r)
		}
	}
	return snapIDs, chunkHashes, refs, nil
}

func collectPullScope(ctx context.Context, rfs RemoteFS, branch string) ([]core.SnapshotID, []core.Hash, []*core.Reference, error) {
	// For pull, we need to list remote directories.
	// First collect remote refs (to know branch tips).
	refs, err := listRemoteRefs(ctx, rfs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list remote refs: %w", err)
	}

	var snapIDs []core.SnapshotID
	var chunkHashes []core.Hash
	var refsToSync []*core.Reference

	if branch != "" {
		// Branch-scoped: only sync the specified branch.
		refName := "heads/" + branch
		var found *core.Reference
		for _, r := range refs {
			if r.Name == refName {
				found = r
				break
			}
		}
		if found == nil {
			return nil, nil, nil, fmt.Errorf("branch %q not found on remote: %w", branch, os.ErrNotExist)
		}
		refsToSync = append(refsToSync, found)
		// Walk the remote snapshot chain to collect all snapshots + chunks.
		ids, chunks, err := walkRemoteSnapshotChain(ctx, rfs, core.SnapshotID{Hash: found.Target})
		if err != nil {
			return nil, nil, nil, err
		}
		snapIDs = ids
		chunkHashes = chunks
	} else {
		// Full repo: list all remote manifests, collect all refs.
		ids, err := listRemoteSnapshots(ctx, rfs)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("list remote snapshots: %w", err)
		}
		snapIDs = ids
		// Collect chunks from each remote snapshot.
		seenChunks := make(map[core.Hash]bool)
		for _, id := range ids {
			snap, err := readRemoteSnapshot(ctx, rfs, id)
			if err != nil {
				return nil, nil, nil, err
			}
			for _, f := range snap.Files {
				for _, ch := range f.Chunks {
					if !seenChunks[ch] {
						seenChunks[ch] = true
						chunkHashes = append(chunkHashes, ch)
					}
				}
			}
		}
		refsToSync = refs
	}
	return snapIDs, chunkHashes, refsToSync, nil
}

func walkSnapshotChain(ctx context.Context, store storage.Storer, start core.SnapshotID) ([]core.SnapshotID, []core.Hash, error) {
	var snapIDs []core.SnapshotID
	var chunkHashes []core.Hash
	seen := make(map[core.Hash]bool)
	seenChunks := make(map[core.Hash]bool)
	cur := start
	for !cur.Hash.IsZero() {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if seen[cur.Hash] {
			break // cycle guard
		}
		seen[cur.Hash] = true
		snapIDs = append(snapIDs, cur)
		snap, err := store.GetSnapshot(ctx, cur)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				break
			}
			return nil, nil, fmt.Errorf("get snapshot %s: %w", cur.Hash.String(), err)
		}
		for _, f := range snap.Files {
			for _, ch := range f.Chunks {
				if !seenChunks[ch] {
					seenChunks[ch] = true
					chunkHashes = append(chunkHashes, ch)
				}
			}
		}
		if snap.PrevID == nil {
			break
		}
		cur = *snap.PrevID
	}
	return snapIDs, chunkHashes, nil
}

func walkRemoteSnapshotChain(ctx context.Context, rfs RemoteFS, start core.SnapshotID) ([]core.SnapshotID, []core.Hash, error) {
	var snapIDs []core.SnapshotID
	var chunkHashes []core.Hash
	seen := make(map[core.Hash]bool)
	seenChunks := make(map[core.Hash]bool)
	cur := start
	for !cur.Hash.IsZero() {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if seen[cur.Hash] {
			break
		}
		seen[cur.Hash] = true
		snapIDs = append(snapIDs, cur)
		snap, err := readRemoteSnapshot(ctx, rfs, cur)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			return nil, nil, err
		}
		for _, f := range snap.Files {
			for _, ch := range f.Chunks {
				if !seenChunks[ch] {
					seenChunks[ch] = true
					chunkHashes = append(chunkHashes, ch)
				}
			}
		}
		if snap.PrevID == nil {
			break
		}
		cur = *snap.PrevID
	}
	return snapIDs, chunkHashes, nil
}

func listRemoteSnapshots(ctx context.Context, rfs RemoteFS) ([]core.SnapshotID, error) {
	// List manifests/ directory (two-level: manifests/ab/cdef...).
	// Walk two levels: manifests/ -> ab/ -> cdef...
	var ids []core.SnapshotID
	level1, err := rfs.List(filesystem.ManifestsDir)
	if err != nil {
		return nil, fmt.Errorf("list manifests: %w", err)
	}
	for _, d1 := range level1 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !d1.IsDir {
			continue
		}
		level2, err := rfs.List(d1.Path)
		if err != nil {
			continue
		}
		for _, d2 := range level2 {
			if d2.IsDir {
				continue
			}
			// Path is like /manifests/ab/cdef..., extract the hash.
			name := path.Base(d2.Path)
			parent := path.Base(path.Dir(d2.Path))
			hexStr := parent + name
			if len(hexStr) != core.HashSize*2 {
				continue
			}
			h, err := parseHashHex(hexStr)
			if err != nil {
				continue
			}
			ids = append(ids, core.SnapshotID{Hash: h})
		}
	}
	return ids, nil
}

func readRemoteSnapshot(ctx context.Context, rfs RemoteFS, id core.SnapshotID) (*core.Snapshot, error) {
	rc, err := rfs.Read(snapshotRemotePath(id))
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read bytes: %w", err)
	}
	p := &core.SnapshotProto{}
	if err := proto.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return core.SnapshotFromProto(p), nil
}

func listRemoteRefs(ctx context.Context, rfs RemoteFS) ([]*core.Reference, error) {
	var refs []*core.Reference
	// List refs/heads/ and refs/tags/.
	for _, sub := range []string{filesystem.RefsDir + "/" + filesystem.HeadsDir, filesystem.RefsDir + "/" + filesystem.TagsDir} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, err := rfs.List(sub)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("list %s: %w", sub, err)
		}
		for _, e := range entries {
			if e.IsDir {
				continue
			}
			// Path like /refs/heads/main → ref name "heads/main"
			rel := strings.TrimPrefix(e.Path, "/")
			rel = strings.TrimPrefix(rel, filesystem.RefsDir+"/")
			rc, err := rfs.Read(e.Path)
			if err != nil {
				continue
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			content := strings.TrimSpace(string(data))
			h, err := parseHashHex(content)
			if err != nil {
				continue
			}
			refs = append(refs, &core.Reference{
				Name:   rel,
				Target: h,
			})
		}
	}
	return refs, nil
}

func refRemotePath(name string) string {
	return path.Join(filesystem.RefsDir, name)
}

// parseHashHex decodes a 64-character hex string into a Hash.
func parseHashHex(s string) (core.Hash, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return core.Hash{}, err
	}
	if len(b) != core.HashSize {
		return core.Hash{}, fmt.Errorf("hash length %d, want %d", len(b), core.HashSize)
	}
	var h core.Hash
	copy(h[:], b)
	return h, nil
}

// --- zstd codec (shared, lazily initialized) ---

var (
	zstdEncOnce sync.Once
	zstdEnc     *zstd.Encoder
	zstdEncErr  error

	zstdDecOnce sync.Once
	zstdDec     *zstd.Decoder
	zstdDecErr  error
)

func zstdEncoder() (*zstd.Encoder, error) {
	zstdEncOnce.Do(func() {
		// zstd.NewWriter returns *zstd.Encoder (klauspost/compress naming);
		// it is NOT called NewEncoder despite the type name.
		enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			zstdEncErr = err
			return
		}
		zstdEnc = enc
	})
	return zstdEnc, zstdEncErr
}

func zstdDecoder() (*zstd.Decoder, error) {
	zstdDecOnce.Do(func() {
		zstdDec, zstdDecErr = zstd.NewReader(nil)
	})
	return zstdDec, zstdDecErr
}

// --- index rebuild + branch helpers ---

func rebuildIndex(ctx context.Context, store storage.Storer, tip core.SnapshotID) error {
	snap, err := store.GetSnapshot(ctx, tip)
	if err != nil {
		return fmt.Errorf("get snapshot: %w", err)
	}
	newIndex := &core.Index{UpdatedAt: time.Now().Unix()}
	for _, entry := range snap.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		newIndex.Entries = append(newIndex.Entries, core.IndexEntry{
			Path:    entry.Path,
			Size:    entry.Size,
			ModTime: entry.ModTime,
			Chunks:  entry.Chunks,
			Hash:    entry.Hash,
		})
	}
	return store.SetIndex(ctx, newIndex)
}

func currentBranchName(ctx context.Context, store storage.Storer) (string, error) {
	head, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		return "", err
	}
	if head.SymRef == "" {
		return "", fmt.Errorf("HEAD is not a symbolic ref")
	}
	return head.SymRef, nil
}

func currentBranchTip(ctx context.Context, store storage.Storer) (core.SnapshotID, error) {
	name, err := currentBranchName(ctx, store)
	if err != nil {
		return core.SnapshotID{}, err
	}
	ref, err := store.GetRef(ctx, name)
	if err != nil {
		return core.SnapshotID{}, err
	}
	return core.SnapshotID{Hash: ref.Target}, nil
}
