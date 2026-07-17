package core

// FileTree is an independent, content-addressed file listing. Separating
// the file listing from Snapshot metadata enables:
//   - ListSnapshots without loading file lists
//   - Cross-snapshot sharing of identical file trees
//   - Incremental tree updates (only changed subtrees need new objects)
//
// In the transition from the embedded model, FileTree provides a clean
// path: new code writes trees independently, old code reads snapshots
// with embedded Files as before.
type FileTree struct {
	ID      Hash
	Entries []FileEntry
}

// SnapshotToFileTree converts a snapshot's embedded file list into a
// standalone FileTree. The TreeID matches the snapshot's ID because the
// snapshot ID is computed from the proto-encoded entries.
func SnapshotToFileTree(s *Snapshot) *FileTree {
	return &FileTree{
		ID:      s.ID.Hash,
		Entries: s.Files,
	}
}
