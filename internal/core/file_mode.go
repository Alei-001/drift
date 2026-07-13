package core

// FileMode represents a file's mode/type.
type FileMode uint32

const (
	// FileModeMask is the bitmask for file type bits.
	FileModeMask FileMode = 0o170000

	// File type bits (use with FileModeMask for type comparison)
	FileModeRegular    FileMode = 0o100000
	FileModeDir        FileMode = 0o040000
	FileModeSymlink    FileMode = 0o120000
	FileModeDevice     FileMode = 0o020000 // block or character device
	FileModeNamedPipe  FileMode = 0o010000 // FIFO (named pipe)
	FileModeSocket     FileMode = 0o140000 // socket
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
	case FileModeDevice:
		return "device"
	case FileModeNamedPipe:
		return "named-pipe"
	case FileModeSocket:
		return "socket"
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

// IsDevice returns true if the file mode represents a block or character
// device.
func (m FileMode) IsDevice() bool {
	return m&FileModeMask == FileModeDevice
}

// IsNamedPipe returns true if the file mode represents a FIFO (named pipe).
func (m FileMode) IsNamedPipe() bool {
	return m&FileModeMask == FileModeNamedPipe
}

// IsSocket returns true if the file mode represents a socket.
func (m FileMode) IsSocket() bool {
	return m&FileModeMask == FileModeSocket
}
