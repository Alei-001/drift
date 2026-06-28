package core

// IndexEntry represents a single file entry in the staging index.
type IndexEntry struct {
	Path    string
	Hash    Hash
	Size    int64
	ModTime int64
	Chunks  []Hash
}

// Index represents the staging area for tracking file changes.
type Index struct {
	Entries   []IndexEntry
	UpdatedAt int64
}

// Find looks up an index entry by path. Returns the entry and true if found.
func (idx *Index) Find(path string) (*IndexEntry, bool) {
	for i := range idx.Entries {
		if idx.Entries[i].Path == path {
			return &idx.Entries[i], true
		}
	}
	return nil, false
}
