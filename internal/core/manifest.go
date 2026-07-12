package core

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
