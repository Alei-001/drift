package core

// FileMode represents a file's mode/type.
type FileMode uint32

const (
	FileModeRegular FileMode = 0100644
	FileModeDir     FileMode = 0040000
	FileModeSymlink FileMode = 0120000
)

// String returns a human-readable representation of the file mode.
func (m FileMode) String() string {
	switch m {
	case FileModeRegular:
		return "regular"
	case FileModeDir:
		return "dir"
	case FileModeSymlink:
		return "symlink"
	default:
		return "unknown"
	}
}

// IsRegular returns true if the file mode represents a regular file.
func (m FileMode) IsRegular() bool {
	return m == FileModeRegular
}

// IsDir returns true if the file mode represents a directory.
func (m FileMode) IsDir() bool {
	return m == FileModeDir
}
