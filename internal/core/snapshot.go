package core

// SnapshotID uniquely identifies a snapshot by its hash.
type SnapshotID struct {
	Hash Hash
}

// Snapshot represents a point-in-time version of the tracked files.
type Snapshot struct {
	ID        SnapshotID
	PrevID    *SnapshotID // nil for the first commit
	Message   string
	Author    string
	Timestamp int64 // unix timestamp
	Files     []FileEntry
	Tags      []string
	TotalSize int64
}

// ShortID returns the short (8-char) hex representation of the snapshot ID.
func (s *Snapshot) ShortID() string {
	return s.ID.Hash.String()
}

// FullID returns the full (64-char) hex representation of the snapshot ID.
func (s *Snapshot) FullID() string {
	return s.ID.Hash.FullString()
}

// SnapshotSummary is lightweight snapshot metadata without a file list.
// ListSnapshots returns summaries so callers that need file details must
// call GetSnapshot with the summary's ID.
type SnapshotSummary struct {
	ID        SnapshotID
	PrevID    *SnapshotID // nil for the first commit
	Message   string
	Author    string
	Timestamp int64 // unix timestamp
	Tags      []string
	TotalSize int64
}

// ShortID returns the short (8-char) hex representation of the snapshot ID.
func (ss *SnapshotSummary) ShortID() string {
	return ss.ID.Hash.String()
}

// FullID returns the full (64-char) hex representation of the snapshot ID.
func (ss *SnapshotSummary) FullID() string {
	return ss.ID.Hash.FullString()
}
