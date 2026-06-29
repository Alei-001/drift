package core

// FileMode represents a file's mode/type.
type FileMode uint32

const (
	// FileModeMask is the bitmask for file type bits.
	FileModeMask FileMode = 0o170000

	// File type bits (use with FileModeMask for type comparison)
	FileModeRegular FileMode = 0o100000
	FileModeDir     FileMode = 0o040000
	FileModeSymlink FileMode = 0o120000

	// FileModeRegularDefault is the default mode for regular files (type + 0644 perms).
	FileModeRegularDefault FileMode = 0o100644
)

// String returns a human-readable representation of the file mode.
func (m FileMode) String() string {
	switch m & FileModeMask {
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
	return m&FileModeMask == FileModeRegular
}

// IsDir returns true if the file mode represents a directory.
func (m FileMode) IsDir() bool {
	return m&FileModeMask == FileModeDir
}

// IsSymlink returns true if the file mode represents a symbolic link.
func (m FileMode) IsSymlink() bool {
	return m&FileModeMask == FileModeSymlink
}

// Perm returns the permission bits of the file mode.
func (m FileMode) Perm() FileMode {
	return m & 0o777
}
