package core

// FileMetadata holds optional metadata about a file.
type FileMetadata struct {
	MIMEType string
	Extra    map[string]string
}

// FileEntry describes a file tracked in a snapshot.
type FileEntry struct {
	Path     string
	Mode     FileMode
	Size     int64
	ModTime  int64 // unix timestamp in nanoseconds (time.UnixNano())
	Chunks   []Hash
	Hash     Hash
	Metadata *FileMetadata
}
