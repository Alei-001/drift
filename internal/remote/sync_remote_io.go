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

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/klauspost/compress/zstd"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
)

// --- path helpers ---

func snapshotRemotePath(id core.SnapshotID) string {
	h := id.Hash.FullString()
	return path.Join(storage.SnapshotsDir, h[:2], h[2:])
}

func manifestRemotePath(id core.SnapshotID) string {
	h := id.Hash.FullString()
	return path.Join(storage.ManifestsDir, h[:2], h[2:])
}

func chunkRemotePath(h core.Hash) string {
	hex := h.FullString()
	return path.Join(storage.ChunksDir, hex[:2], hex[2:])
}

func refRemotePath(name string) string {
	return path.Join(storage.RefsDir, name)
}

// --- push object helpers ---

// pushSnapshot uploads a single snapshot to the remote. The caller should
// first check rfs.Stat(snapshotRemotePath(id)) to skip snapshots already
// present; pushSnapshot itself always (over)writes, which is safe for
// content-addressed storage but wasteful if the object already exists.
func pushSnapshot(ctx context.Context, store storage.Storer, rfs RemoteFS, id core.SnapshotID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	snap, err := store.GetSnapshot(ctx, id)
	if err != nil {
		return fmt.Errorf("get snapshot: %w", err)
	}
	p := core.SnapshotToProto(snap, true)
	data, err := proto.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	return rfs.Write(ctx, snapshotRemotePath(id), bytes.NewReader(data))
}

// pushManifest uploads the lightweight manifest derived from a snapshot.
// The manifest is small, so callers may upload it unconditionally even when
// the snapshot already exists on the remote (see P1-9: manifest existence
// is checked independently of snapshot existence).
func pushManifest(ctx context.Context, store storage.Storer, rfs RemoteFS, id core.SnapshotID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	snap, err := store.GetSnapshot(ctx, id)
	if err != nil {
		return fmt.Errorf("get snapshot for manifest: %w", err)
	}
	m := core.SnapshotToManifest(snap)
	data, err := proto.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return rfs.Write(ctx, manifestRemotePath(id), bytes.NewReader(data))
}

func pushChunk(ctx context.Context, store storage.Storer, rfs RemoteFS, h core.Hash) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	chunk, err := store.GetChunk(ctx, h)
	if err != nil {
		return fmt.Errorf("get chunk: %w", err)
	}
	// Re-encode using the chunk's stored flags, matching the local on-disk
	// format: 1-byte header + (optionally compressed) payload.
	var payload []byte
	var header byte
	if chunk.Flags&core.ChunkFlagCompressed != 0 {
		enc, err := acquireZstdEncoder()
		if err != nil {
			return fmt.Errorf("zstd encoder: %w", err)
		}
		defer releaseZstdEncoder(enc)
		compressed := enc.EncodeAll(chunk.Data, nil)
		header = storage.ChunkFlagCompressed
		payload = make([]byte, 0, storage.ChunkHeaderSize+len(compressed))
		payload = append(payload, header)
		payload = append(payload, compressed...)
	} else {
		header = 0
		payload = make([]byte, 0, storage.ChunkHeaderSize+len(chunk.Data))
		payload = append(payload, header)
		payload = append(payload, chunk.Data...)
	}
	return rfs.Write(ctx, chunkRemotePath(h), bytes.NewReader(payload))
}

var errRefDiverged = errors.New("ref diverged")

// ErrNetwork indicates a remote I/O failure (connection refused, timeout,
// DNS failure, etc.). The cmd layer maps it to ExitNetwork so scripts can
// distinguish network failures from other errors.
var ErrNetwork = errors.New("network error")

// pushRef uploads a single ref to the remote. It is fast-forward only: if
// the remote ref already exists and its target is not an ancestor of the
// local target, pushRef returns errRefDiverged and the caller should surface
// a "pull first" message.
//
// Known limitation (TOCTOU): the read-then-write sequence is not atomic
// across processes. Two machines pushing to the same remote ref concurrently
// could race: both read the old target, both pass the fast-forward check, and
// the second write silently overwrites the first. gowebdav does not expose
// ETag/If-Match conditional PUT in a portable way, and go-smb2 has no
// compare-and-swap primitive, so a true CAS is not feasible with the current
// libraries. Mitigation: the workspace lock (AcquireWorkspaceLock in
// porcelain.PushToRemote) serializes push/pull against other local
// workspace-modifying commands, and content-addressed storage means a lost
// ref update only loses the tip pointer (no object corruption). For
// multi-client setups, users should coordinate pushes externally.
func pushRef(ctx context.Context, store storage.Storer, rfs RemoteFS, ref *core.Reference) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	// Only upload ref if its target snapshot exists on the remote.
	snapPath := snapshotRemotePath(core.SnapshotID{Hash: ref.Target})
	if _, err := rfs.Stat(ctx, snapPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil // target snapshot not on remote yet, skip
		}
		return false, fmt.Errorf("stat remote snapshot for ref: %w", err)
	}
	refPath := refRemotePath(ref.Name)
	existing, err := rfs.Read(ctx, refPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("read existing remote ref: %w", err)
		}
		// No existing ref, write it.
		if err := rfs.Write(ctx, refPath, strings.NewReader(ref.Target.FullString()+"\n")); err != nil {
			return false, err
		}
		return true, nil
	}
	defer existing.Close()
	existingBytes, err := io.ReadAll(existing)
	if err != nil {
		return false, fmt.Errorf("read existing remote ref: %w", err)
	}
	existingHashStr := strings.TrimSpace(string(existingBytes))
	if existingHashStr == ref.Target.FullString() {
		return false, nil // same, skip
	}
	// Fast-forward check: if the remote target is an ancestor of the local
	// target, the local branch is simply ahead — allow the push.
	existingHash, parseErr := parseHashHex(existingHashStr)
	if parseErr == nil {
		ok, ancErr := isAncestor(ctx, store, ref.Target, existingHash)
		if ancErr != nil {
			// Cannot determine ancestry (e.g. the local chain is broken
			// or the remote's existing target snapshot is missing from
			// the local store). Surface the underlying error rather than
			// silently collapsing it into "diverged", so the user can
			// distinguish a real divergence from a broken local chain.
			return false, fmt.Errorf("fast-forward check against %s: %w", existingHash.FullString(), ancErr)
		}
		if ok {
			if err := rfs.Write(ctx, refPath, strings.NewReader(ref.Target.FullString()+"\n")); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, errRefDiverged
}

// --- pull object helpers ---

func pullSnapshot(ctx context.Context, store storage.Storer, rfs RemoteFS, id core.SnapshotID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rc, err := rfs.Read(ctx, snapshotRemotePath(id))
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
	// Verify content integrity: re-marshal without IdHash (matching the
	// original ID computation in porcelain) and check the BLAKE3 hash
	// matches the expected snapshot ID. This detects silent corruption
	// from bit rot, transfer errors, or a malicious remote.
	snapForHash, err := core.SnapshotFromProto(p)
	if err != nil {
		return fmt.Errorf("decode snapshot for hash: %w", err)
	}
	hashProto := core.SnapshotToProto(snapForHash, false)
	hashData, err := proto.Marshal(hashProto)
	if err != nil {
		return fmt.Errorf("re-marshal snapshot for hash: %w", err)
	}
	computed := core.Hash(blake3.Sum256(hashData))
	if computed != id.Hash {
		return fmt.Errorf("snapshot %s integrity check failed: expected %s, got %s: %w",
			id.Hash.FullString()[:8], id.Hash.FullString(), computed.FullString(), storage.ErrCorrupted)
	}
	snap, err := core.SnapshotFromProto(p)
	if err != nil {
		return fmt.Errorf("decode snapshot: %w", err)
	}
	snap.ID = id
	if err := store.PutSnapshot(ctx, snap); err != nil {
		return fmt.Errorf("put snapshot: %w", err)
	}
	return nil
}

func pullChunk(ctx context.Context, store storage.Storer, rfs RemoteFS, h core.Hash) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rc, err := rfs.Read(ctx, chunkRemotePath(h))
	if err != nil {
		return fmt.Errorf("read remote chunk: %w", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("read chunk bytes: %w", err)
	}
	if len(data) < storage.ChunkHeaderSize {
		return fmt.Errorf("chunk too short: %w", storage.ErrCorrupted)
	}
	header := data[0]
	payload := data[storage.ChunkHeaderSize:]
	var chunkData []byte
	var flags core.ChunkFlag
	if header&storage.ChunkFlagCompressed != 0 {
		dec, err := acquireZstdDecoder()
		if err != nil {
			return fmt.Errorf("zstd decoder: %w", err)
		}
		defer releaseZstdDecoder(dec)
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
	// Verify content integrity: the BLAKE3 hash of the decoded chunk data
	// must match the expected hash. This detects silent corruption from
	// bit rot, transfer errors, or a malicious remote before the chunk
	// enters the local store.
	computed := core.Hash(blake3.Sum256(chunkData))
	if computed != h {
		return fmt.Errorf("chunk %x integrity check failed: expected %s, got %s: %w",
			h[:8], h.FullString(), computed.FullString(), storage.ErrCorrupted)
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
	// Fast-forward: if local target is zero (fresh branch) or an ancestor of
	// the remote target, the remote is simply ahead — update the local ref.
	if localRef.Target.IsZero() {
		if err := store.SetRef(ctx, ref.Name, ref); err != nil {
			return false, false, fmt.Errorf("fast-forward ref: %w", err)
		}
		return true, false, nil
	}
	ok, ancErr := isAncestor(ctx, store, ref.Target, localRef.Target)
	if ancErr != nil {
		// Cannot determine ancestry (e.g. the local chain is broken
		// or a snapshot is missing from the local store). Return an
		// error rather than silently treating the ref as diverged —
		// the user should investigate and retry.
		return false, false, fmt.Errorf("fast-forward check ref %q: %w", ref.Name, ancErr)
	}
	if ok {
		if err := store.SetRef(ctx, ref.Name, ref); err != nil {
			return false, false, fmt.Errorf("fast-forward ref: %w", err)
		}
		return true, false, nil
	}
	// Diverged: save the remote version under a side ref so the local ref is
	// never silently overwritten. If <name>.remote already exists with a
	// different target, disambiguate by appending the short remote hash so a
	// second divergent pull does not clobber the first (P2-e).
	remoteName := ref.Name + ".remote"
	if existing, err := store.GetRef(ctx, remoteName); err == nil {
		if existing.Target != ref.Target {
			remoteName = ref.Name + ".remote." + ref.Target.FullString()[:8]
		}
	} else if !errors.Is(err, storage.ErrNotFound) {
		return false, false, fmt.Errorf("check existing diverged ref: %w", err)
	}
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

// --- remote listing helpers ---

func listRemoteSnapshots(ctx context.Context, rfs RemoteFS) ([]core.SnapshotID, error) {
	// List snapshots/ directory (two-level: snapshots/ab/cdef...). This is
	// the authoritative location for snapshot objects; manifests/ is a
	// secondary index that may lag behind, so listing snapshots/ avoids
	// missing snapshots whose manifest has not been uploaded yet.
	var ids []core.SnapshotID
	level1, err := rfs.List(ctx, storage.SnapshotsDir)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	for _, d1 := range level1 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !d1.IsDir {
			continue
		}
		level2, err := rfs.List(ctx, d1.Path)
		if err != nil {
			// A missing prefix directory is legitimate (no snapshots
			// under that prefix); other errors must not be swallowed.
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("list remote snapshots in %s: %w", d1.Path, err)
		}
		for _, d2 := range level2 {
			if d2.IsDir {
				continue
			}
			// Path is like /snapshots/ab/cdef..., extract the hash.
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rc, err := rfs.Read(ctx, snapshotRemotePath(id))
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
	return core.SnapshotFromProto(p)
}

// listRemoteRefs enumerates all refs on the remote. It lists refs/ one level
// to discover ref categories (heads/, tags/, ...), then lists each category
// directory. This avoids hardcoding "heads"/"tags" and supports any future
// ref category. Ref names are derived from path.Base of each entry so the
// extraction is robust to a subPath prefix that SMB prepends internally
// (P1-10): previously the code used strings.TrimPrefix on the absolute path,
// which broke when SMB resolved a subPath into e.Path.
func listRemoteRefs(ctx context.Context, rfs RemoteFS) ([]*core.Reference, error) {
	var refs []*core.Reference
	top, err := rfs.List(ctx, storage.RefsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return refs, nil
		}
		return nil, fmt.Errorf("list %s: %w", storage.RefsDir, err)
	}
	for _, d1 := range top {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !d1.IsDir {
			continue
		}
		category := path.Base(d1.Path)
		entries, err := rfs.List(ctx, d1.Path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("list remote refs in %s: %w", d1.Path, err)
		}
		for _, e := range entries {
			if e.IsDir {
				continue
			}
			// Ref name is "<category>/<entry>". Using path.Base(e.Path) for
			// the entry name makes this independent of any subPath prefix.
			refName := category + "/" + path.Base(e.Path)
			rc, err := rfs.Read(ctx, e.Path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return nil, fmt.Errorf("read remote ref %s: %w", refName, err)
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("read ref body %s: %w", refName, err)
			}
			content := strings.TrimSpace(string(data))
			h, err := parseHashHex(content)
			if err != nil {
				continue
			}
			refs = append(refs, &core.Reference{
				Name:   refName,
				Target: h,
			})
		}
	}
	return refs, nil
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

// --- zstd codec (pooled for concurrency safety) ---

// zstdEncPool and zstdDecPool provide per-goroutine zstd codec instances.
// Although zstd.Encoder.EncodeAll and zstd.Decoder.DecodeAll are documented
// as concurrency-safe, using a sync.Pool avoids any contention on shared
// internal state and is the recommended pattern for concurrent callers
// (pushChunksConcurrent / pullChunksConcurrent worker pools).
var zstdEncPool = sync.Pool{
	New: func() any {
		enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			// Only fails with invalid options (hardcoded here), so wrap
			// the error for the caller. In practice this never triggers.
			return zstdPoolError{err: err}
		}
		return enc
	},
}

// zstdDecPool is the decoder counterpart of zstdEncPool.
var zstdDecPool = sync.Pool{
	New: func() any {
		dec, err := zstd.NewReader(nil)
		if err != nil {
			return zstdPoolError{err: err}
		}
		return dec
	},
}

// zstdPoolError wraps a codec-init error stored in a sync.Pool slot.
type zstdPoolError struct{ err error }

func (e zstdPoolError) Error() string { return e.err.Error() }

// acquireZstdEncoder borrows an encoder from the pool.
func acquireZstdEncoder() (*zstd.Encoder, error) {
	v := zstdEncPool.Get()
	if pe, ok := v.(zstdPoolError); ok {
		return nil, pe.err
	}
	return v.(*zstd.Encoder), nil
}

// releaseZstdEncoder returns an encoder to the pool for reuse.
func releaseZstdEncoder(enc *zstd.Encoder) {
	zstdEncPool.Put(enc)
}

// acquireZstdDecoder borrows a decoder from the pool.
func acquireZstdDecoder() (*zstd.Decoder, error) {
	v := zstdDecPool.Get()
	if pe, ok := v.(zstdPoolError); ok {
		return nil, pe.err
	}
	return v.(*zstd.Decoder), nil
}

// releaseZstdDecoder returns a decoder to the pool for reuse.
func releaseZstdDecoder(dec *zstd.Decoder) {
	zstdDecPool.Put(dec)
}
