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
	"github.com/Alei-001/drift/internal/storage/backends/filesystem"
	"github.com/klauspost/compress/zstd"
	"google.golang.org/protobuf/proto"
)

// chunkHeaderSize and chunkFlagCompressed mirror the constants in
// filesystem/chunk.go. They are duplicated here because sync needs to
// re-encode chunks for upload (push) and decode them on download (pull),
// without depending on the filesystem package's unexported constants.
// The remote wire format MUST match the local on-disk format.
const (
	chunkHeaderSize          = 1
	chunkFlagCompressed byte = 0x01
)

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

func refRemotePath(name string) string {
	return path.Join(filesystem.RefsDir, name)
}

// --- push object helpers ---

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

func pushRef(ctx context.Context, store storage.Storer, rfs RemoteFS, ref *core.Reference) (bool, error) {
	// Only upload ref if its target snapshot exists on the remote.
	snapPath := snapshotRemotePath(core.SnapshotID{Hash: ref.Target})
	if _, err := rfs.Stat(snapPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil // target snapshot not on remote yet, skip
		}
		return false, fmt.Errorf("stat remote snapshot for ref: %w", err)
	}
	refPath := refRemotePath(ref.Name)
	existing, err := rfs.Read(refPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("read existing remote ref: %w", err)
		}
		// No existing ref, write it.
		if err := rfs.Write(refPath, strings.NewReader(ref.Target.FullString()+"\n")); err != nil {
			return false, err
		}
		return true, nil
	}
	defer existing.Close()
	existingBytes, _ := io.ReadAll(existing)
	existingHashStr := strings.TrimSpace(string(existingBytes))
	if existingHashStr == ref.Target.FullString() {
		return false, nil // same, skip
	}
	// Fast-forward check: if the remote target is an ancestor of the local
	// target, the local branch is simply ahead — allow the push.
	existingHash, err := parseHashHex(existingHashStr)
	if err == nil && isAncestor(ctx, store, ref.Target, existingHash) {
		if err := rfs.Write(refPath, strings.NewReader(ref.Target.FullString()+"\n")); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, errRefDiverged
}

// --- pull object helpers ---

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
	// Fast-forward: if local target is zero (fresh branch) or an ancestor of
	// the remote target, the remote is simply ahead — update the local ref.
	if localRef.Target.IsZero() || isAncestor(ctx, store, ref.Target, localRef.Target) {
		if err := store.SetRef(ctx, ref.Name, ref); err != nil {
			return false, false, fmt.Errorf("fast-forward ref: %w", err)
		}
		return true, false, nil
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

// --- remote listing helpers ---

func listRemoteSnapshots(ctx context.Context, rfs RemoteFS) ([]core.SnapshotID, error) {
	// List manifests/ directory (two-level: manifests/ab/cdef...).
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
