package core

// FileMetadata holds optional metadata about a file.
type FileMetadata struct {
	MimeType string
	Extra    map[string]string
}

// FileEntry describes a file tracked in a snapshot.
type FileEntry struct {
	Path     string
	Mode     FileMode
	Size     int64
	ModTime  int64 // unix timestamp
	Chunks   []Hash
	Metadata *FileMetadata
}
