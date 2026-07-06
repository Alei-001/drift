package core

import (
	"google.golang.org/protobuf/proto"
)

func SnapshotToManifest(s *Snapshot) *SnapshotManifest {
	if s == nil {
		return nil
	}
	m := &SnapshotManifest{
		Id:           make([]byte, HashSize),
		Message:      s.Message,
		Author:       s.Author,
		Timestamp:    s.Timestamp,
		Tags:         s.Tags,
		TotalSize:    s.TotalSize,
		FilesChanged: int32(len(s.Files)),
	}
	copy(m.Id, s.ID.Hash[:])
	if s.PrevID != nil {
		prev := make([]byte, HashSize)
		copy(prev, s.PrevID.Hash[:])
		m.PrevId = prev
	}
	return m
}

// ManifestToSummary converts a snapshot manifest to a lightweight summary.
func ManifestToSummary(m *SnapshotManifest) *SnapshotSummary {
	if m == nil {
		return nil
	}
	var id SnapshotID
	copy(id.Hash[:], m.Id)
	ss := &SnapshotSummary{
		ID:        id,
		Message:   m.Message,
		Author:    m.Author,
		Timestamp: m.Timestamp,
		Tags:      m.Tags,
		TotalSize: m.TotalSize,
	}
	if len(m.PrevId) >= HashSize {
		prev := &SnapshotID{}
		copy(prev.Hash[:], m.PrevId)
		ss.PrevID = prev
	}
	return ss
}

func ManifestToSnapshot(m *SnapshotManifest) *Snapshot {
	if m == nil {
		return nil
	}
	var id SnapshotID
	copy(id.Hash[:], m.Id)
	s := &Snapshot{
		ID:        id,
		Message:   m.Message,
		Author:    m.Author,
		Timestamp: m.Timestamp,
		Tags:      m.Tags,
		TotalSize: m.TotalSize,
	}
	if m.PrevId != nil {
		prev := &SnapshotID{}
		copy(prev.Hash[:], m.PrevId)
		s.PrevID = prev
	}
	return s
}

func MarshalManifest(m *SnapshotManifest) ([]byte, error) {
	return proto.Marshal(m)
}

func UnmarshalManifest(data []byte, m *SnapshotManifest) error {
	return proto.Unmarshal(data, m)
}
