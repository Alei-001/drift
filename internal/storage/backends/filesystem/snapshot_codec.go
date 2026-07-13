package filesystem

import (
	"encoding/hex"
	"path/filepath"

	"github.com/Alei-001/drift/internal/core"
)

// snapshotIDFromPath extracts the SnapshotID from a snapshot file path.
// Path layout: <root>/snapshots/<hex[:2]>/<hex[2:]>.
func snapshotIDFromPath(path string) (core.SnapshotID, bool) {
	name := filepath.Base(path)
	parent := filepath.Base(filepath.Dir(path))
	hexStr := parent + name
	if len(hexStr) != core.HashSize*2 {
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
// Delegates to core.SnapshotFromProto, which is shared with the remote sync
// layer so both decode persisted snapshots identically.
func snapshotFromProto(p *core.SnapshotProto) (*core.Snapshot, error) {
	return core.SnapshotFromProto(p)
}

// bytesToHash delegates to core.BytesToHash. Kept as an unexported wrapper
// so index.go and other filesystem-internal callers can use a short name.
func bytesToHash(b []byte) core.Hash {
	return core.BytesToHash(b)
}

// copyBytes returns a defensive copy of b so callers cannot mutate the
// original. Shared between snapshot and index protobuf encoding.
func copyBytes(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
