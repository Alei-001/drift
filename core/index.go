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
