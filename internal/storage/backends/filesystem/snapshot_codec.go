package filesystem

import (
	"encoding/hex"
	"path/filepath"

	"github.com/your-org/drift/internal/core"
)

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

// manifestID returns the SnapshotID encoded in a manifest.
func manifestID(m *core.SnapshotManifest) core.SnapshotID {
	var id core.SnapshotID
	copy(id.Hash[:], m.Id)
	return id
}

// snapshotFromProto rebuilds a core.Snapshot from its protobuf wire form.
// snapshotToProto lives in the core package (core.SnapshotToProto) so the
// porcelain and storage layers share a single, drift-stable serialization;
// snapshotFromProto stays in the storage layer because only the storage
// layer needs to decode persisted snapshots.
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

// bytesToHash copies a byte slice into a Hash. The caller must ensure len(b)
// <= HashSize; extra bytes are truncated (only the first HashSize bytes are
// copied). Used when decoding protobuf IdHash/PrevIdHash/ChunkHashes fields.
func bytesToHash(b []byte) core.Hash {
	var h core.Hash
	copy(h[:], b)
	return h
}

// copyBytes returns a defensive copy of b so callers cannot mutate the
// original. Shared between snapshot and index protobuf encoding.
func copyBytes(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
